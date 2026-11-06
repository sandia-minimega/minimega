// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/sys/unix"
)

const (
	cgroupRoot = "/sys/fs/cgroup"

	// fcIDRange is the size of the uid and gid range for Firecracker VMMs.
	fcIDRange = 65536

	fcNetnsDir = "/var/run/netns"
)

// fcMinVersion is the oldest Firecracker release that minimega supports.
var fcMinVersion = fcVersion{1, 17, 0}

// fcRunVersion runs `<path> --version` and returns its output. Tests replace
// it.
var fcRunVersion = func(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	return string(out), err
}

// fcPermStat reads the permissions of a binary and its parents. Tests replace
// it.
var fcPermStat = os.Stat

var (
	fcBinaryCache sync.Map // resolved path -> *fcBinaryEntry
	fcDirCache    sync.Map // clean path -> *fcDirEntry
)

type fcVersion struct {
	Major, Minor, Patch int
}

func (v fcVersion) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v fcVersion) Less(o fcVersion) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}

// fcBinaryInfo holds the facts about a Firecracker or jailer binary. Unsafe
// lists the binary and parent directories that the group or others can write
// to.
type fcBinaryInfo struct {
	Path    string
	Version fcVersion
	Static  bool
	Unsafe  []string
	Err     error
}

type fcBinaryEntry struct {
	once sync.Once
	info fcBinaryInfo
}

// fcDirInfo holds the facts about the -firecracker-dir directory.
type fcDirInfo struct {
	Path       string
	Exists     bool
	FSType     string
	NoDev      bool
	NoExec     bool
	Reflink    bool
	ReflinkErr error
	Unsafe     []string
	Err        error
}

type fcDirEntry struct {
	once sync.Once
	info fcDirInfo
}

// Problems returns the reasons that the directory cannot hold jails.
func (d fcDirInfo) Problems() []string {
	var res []string
	if d.Err != nil {
		res = append(res, d.Err.Error())
	}
	if d.NoDev {
		res = append(res, fmt.Sprintf("%v is on a nodev mount", d.Path))
	}
	if d.NoExec {
		res = append(res, fmt.Sprintf("%v is on a noexec mount", d.Path))
	}
	return append(res, d.Unsafe...)
}

var (
	// hostCgroupVersion is the cgroup version of the host, 1 or 2. It is set
	// once at startup because it cannot change while minimega runs.
	hostCgroupVersion int

	// fcInstance is a short name for this minimega instance, derived from
	// -base. It keeps the names of host objects from two instances apart.
	fcInstance string
)

// firecrackerHostInit caches the host facts that Firecracker VMs use. It does
// not create any file, directory, or cgroup.
func firecrackerHostInit() {
	hostCgroupVersion = detectCgroupVersion(unix.Statfs)
	log.Debug("host cgroup version: %v", hostCgroupVersion)

	fcInstance = instanceName(*f_base)
	log.Debug("firecracker instance name: %v", fcInstance)

	base := uint64(*f_firecrackerUIDBase)
	for _, f := range []struct {
		path   string
		fields []int
	}{
		{"/etc/passwd", []int{2, 3}},
		{"/etc/group", []int{2}},
	} {
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}

		if ids := fcIDOverlaps(string(data), base, f.fields...); len(ids) > 0 {
			log.Warn("%v has IDs in the Firecracker uid range %v-%v: %v", f.path, base, base+fcIDRange-1, strings.Join(ids, ", "))
		}
	}
}

// fcIDOverlaps returns "name:id" for each entry of a passwd or group file
// with an ID, at one of the field indexes, in [base, base+fcIDRange).
func fcIDOverlaps(data string, base uint64, fields ...int) []string {
	var res []string

	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, ":")
		for _, i := range fields {
			if i >= len(parts) {
				continue
			}

			id, err := strconv.ParseUint(parts[i], 10, 64)
			if err != nil {
				continue
			}

			if id >= base && id < base+fcIDRange {
				res = append(res, fmt.Sprintf("%v:%v", parts[0], id))
				break
			}
		}
	}

	return res
}

var fcVersionRe = regexp.MustCompile(`v(\d+)\.(\d+)\.(\d+)`)

// parseFCVersion returns the first v<major>.<minor>.<patch> in out.
func parseFCVersion(out string) (fcVersion, error) {
	m := fcVersionRe.FindStringSubmatch(out)
	if m == nil {
		return fcVersion{}, fmt.Errorf("no version in output: %q", strings.TrimSpace(out))
	}

	var v fcVersion
	for i, p := range []*int{&v.Major, &v.Minor, &v.Patch} {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return fcVersion{}, fmt.Errorf("invalid version %q: %v", m[0], err)
		}
		*p = n
	}

	return v, nil
}

// elfIsStatic returns true when the ELF file has no PT_INTERP program header
// and no DT_NEEDED entries.
func elfIsStatic(r io.ReaderAt) (bool, error) {
	f, err := elf.NewFile(r)
	if err != nil {
		return false, err
	}
	defer f.Close()

	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			return false, nil
		}
	}

	libs, err := f.ImportedLibraries()
	if err != nil {
		return false, err
	}

	return len(libs) == 0, nil
}

// fcUnsafePerms returns a problem for path and each parent directory that the
// group or others can write to.
func fcUnsafePerms(path string, stat func(string) (os.FileInfo, error)) []string {
	var res []string

	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		fi, err := stat(p)
		if err != nil {
			res = append(res, fmt.Sprintf("unable to check permissions of %v: %v", p, err))
		} else if fi.Mode().Perm()&0o022 != 0 {
			res = append(res, fmt.Sprintf("%v is writable by group or others (mode %v)", p, fi.Mode().Perm()))
		}

		if p == filepath.Dir(p) {
			break
		}
	}

	return res
}

func fcResolve(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	return path
}

// fcBinary returns the facts about a binary, a path or a name in PATH. The
// facts are computed once for each resolved path.
func fcBinary(name string) fcBinaryInfo {
	path, err := exec.LookPath(name)
	if err != nil {
		return fcBinaryInfo{Path: name, Err: fmt.Errorf("%v not found: %v", name, err)}
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	resolved := fcResolve(path)

	v, _ := fcBinaryCache.LoadOrStore(resolved, &fcBinaryEntry{})
	e := v.(*fcBinaryEntry)
	e.once.Do(func() {
		e.info = fcProbeBinary(path, resolved)
	})

	return e.info
}

func fcProbeBinary(path, resolved string) fcBinaryInfo {
	info := fcBinaryInfo{Path: path}

	f, err := os.Open(resolved)
	if err != nil {
		info.Err = err
		return info
	}
	defer f.Close()

	if info.Static, err = elfIsStatic(f); err != nil {
		info.Err = fmt.Errorf("%v is not a valid ELF binary: %v", path, err)
		return info
	}

	out, err := fcRunVersion(resolved)
	if err != nil {
		info.Err = fmt.Errorf("%v --version failed: %v", path, err)
		return info
	}

	if info.Version, err = parseFCVersion(out); err != nil {
		info.Err = fmt.Errorf("%v --version: %v", path, err)
		return info
	}

	info.Unsafe = fcUnsafePerms(resolved, fcPermStat)

	return info
}

// fcCheckBinaries checks that the Firecracker and jailer binaries can run VMs.
func fcCheckBinaries(fcPath, jailerPath string) (fc, jailer fcBinaryInfo, err error) {
	fc = fcBinary(fcPath)
	jailer = fcBinary(jailerPath)

	var errs []error

	if fc.Err != nil {
		errs = append(errs, fmt.Errorf("firecracker: %v", fc.Err))
	} else {
		if fc.Version.Less(fcMinVersion) {
			errs = append(errs, fmt.Errorf("firecracker %v is %v; minimega needs %v or later", fc.Path, fc.Version, fcMinVersion))
		}
		if !fc.Static {
			errs = append(errs, fmt.Errorf("firecracker %v is not statically linked; the jailer needs a static binary", fc.Path))
		}
		for _, u := range fc.Unsafe {
			errs = append(errs, fmt.Errorf("firecracker: %v", u))
		}
	}

	if jailer.Err != nil {
		errs = append(errs, fmt.Errorf("jailer: %v", jailer.Err))
	} else {
		if fc.Err == nil && jailer.Version != fc.Version {
			errs = append(errs, fmt.Errorf("jailer %v is %v, but firecracker %v is %v; they must be the same version", jailer.Path, jailer.Version, fc.Path, fc.Version))
		}
		for _, u := range jailer.Unsafe {
			errs = append(errs, fmt.Errorf("jailer: %v", u))
		}
	}

	return fc, jailer, errors.Join(errs...)
}

// fcFSTypeName returns the name of a statfs f_type value.
func fcFSTypeName(t int64) string {
	switch t {
	case unix.XFS_SUPER_MAGIC:
		return "xfs"
	case unix.BTRFS_SUPER_MAGIC:
		return "btrfs"
	case unix.EXT4_SUPER_MAGIC:
		return "ext4"
	case unix.TMPFS_MAGIC:
		return "tmpfs"
	}

	return fmt.Sprintf("0x%x", t)
}

// fcCheckDir returns the facts about dir. The facts are computed once for each
// directory that exists.
func fcCheckDir(dir string) fcDirInfo {
	dir = filepath.Clean(dir)

	v, _ := fcDirCache.LoadOrStore(dir, &fcDirEntry{})
	e := v.(*fcDirEntry)
	e.once.Do(func() {
		e.info = fcProbeDir(dir, unix.Statfs, os.Stat)
	})

	// The first launch creates the directory, so check it again then.
	if !e.info.Exists {
		fcDirCache.CompareAndDelete(dir, e)
	}

	return e.info
}

func fcProbeDir(dir string, statfs func(string, *unix.Statfs_t) error, stat func(string) (os.FileInfo, error)) fcDirInfo {
	info := fcDirInfo{Path: dir}

	fi, err := stat(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			info.Err = err
		}
		return info
	}
	if !fi.IsDir() {
		info.Err = fmt.Errorf("%v is not a directory", dir)
		return info
	}
	info.Exists = true

	var st unix.Statfs_t
	if err := statfs(dir, &st); err != nil {
		info.Err = fmt.Errorf("unable to statfs %v: %v", dir, err)
	} else {
		info.FSType = fcFSTypeName(int64(st.Type))
		info.NoDev = st.Flags&unix.ST_NODEV != 0
		info.NoExec = st.Flags&unix.ST_NOEXEC != 0
	}

	info.Unsafe = fcUnsafePerms(fcResolve(dir), stat)
	if _, err := stat(fcNetnsDir); err == nil {
		info.Unsafe = append(info.Unsafe, fcUnsafePerms(fcResolve(fcNetnsDir), stat)...)
	}

	info.Reflink, info.ReflinkErr = fcProbeReflink(dir)

	return info
}

// fcProbeReflink returns true when dir supports FICLONE.
func fcProbeReflink(dir string) (bool, error) {
	src, err := os.CreateTemp(dir, ".reflink-src-")
	if err != nil {
		return false, err
	}
	defer os.Remove(src.Name())
	defer src.Close()

	dst, err := os.CreateTemp(dir, ".reflink-dst-")
	if err != nil {
		return false, err
	}
	defer os.Remove(dst.Name())
	defer dst.Close()

	if _, err := src.Write(make([]byte, 4096)); err != nil {
		return false, err
	}

	return unix.IoctlFileClone(int(dst.Fd()), int(src.Fd())) == nil, nil
}

// checkFirecracker reports the host facts for Firecracker VMs. Firecracker is
// optional, so it only logs.
func checkFirecracker() {
	_, fcErr := exec.LookPath("firecracker")
	_, jailerErr := exec.LookPath("jailer")
	_, dirErr := os.Stat(*f_firecrackerDir)

	if fcErr != nil && jailerErr != nil && dirErr != nil {
		log.Info("firecracker support is not configured: no firecracker or jailer in $PATH and no %v", *f_firecrackerDir)
		return
	}

	fc, jailer, err := fcCheckBinaries("firecracker", "jailer")
	for _, b := range []struct {
		name string
		info fcBinaryInfo
	}{{"firecracker", fc}, {"jailer", jailer}} {
		if b.info.Err == nil {
			log.Info("%v found at: %v, version: %v, static: %v", b.name, b.info.Path, b.info.Version, b.info.Static)
		}
	}
	if err != nil {
		for _, e := range strings.Split(err.Error(), "\n") {
			log.Warn("firecracker unavailable: %v", e)
		}
	} else {
		log.Info("firecracker and jailer versions match: %v", fc.Version)
	}

	log.Info("host cgroup version: %v", hostCgroupVersion)

	if fi, err := os.Stat("/dev/kvm"); err != nil {
		log.Warn("firecracker needs /dev/kvm: %v", err)
	} else if fi.Mode()&os.ModeCharDevice == 0 {
		log.Warn("firecracker needs /dev/kvm, but it is not a character device")
	} else {
		log.Info("/dev/kvm found")
	}

	dir := fcCheckDir(*f_firecrackerDir)
	if !dir.Exists && dir.Err == nil {
		log.Info("%v does not exist; the first firecracker launch creates it", dir.Path)
	} else if dir.Exists {
		log.Info("%v filesystem: %v, reflink: %v", dir.Path, dir.FSType, dir.Reflink)
		if dir.ReflinkErr != nil {
			log.Warn("unable to check reflink support in %v: %v", dir.Path, dir.ReflinkErr)
		}
	}
	for _, p := range dir.Problems() {
		log.Warn("firecracker directory: %v", p)
	}

	for _, p := range []string{"/dev/loop-control", "/dev/mapper/control"} {
		if _, err := os.Stat(p); err != nil {
			log.Warn("%v not found; firecracker needs it for disk snapshots without reflink and for vm config inject", p)
		} else {
			log.Info("%v found", p)
		}
	}

	for _, b := range []struct{ name, use string }{
		{"qemu-img", "to convert qcow2 images"},
		{"dmsetup", "for disk snapshots without reflink"},
		{"setfacl", "for disks with snapshot false"},
	} {
		if path, err := exec.LookPath(b.name); err != nil {
			log.Warn("%v not found in $PATH; firecracker needs it %v", b.name, b.use)
		} else {
			log.Info("%v found at: %v", b.name, path)
		}
	}
}

// detectCgroupVersion returns 2 when the cgroup root is a cgroup2 filesystem,
// else 1. Hybrid hosts use version 1.
func detectCgroupVersion(statfs func(string, *unix.Statfs_t) error) int {
	var st unix.Statfs_t
	if err := statfs(cgroupRoot, &st); err != nil {
		log.Warn("unable to statfs %v, using cgroup version 1: %v", cgroupRoot, err)
		return 1
	}

	if uint64(st.Type) == unix.CGROUP2_SUPER_MAGIC {
		return 2
	}

	return 1
}

// instanceName returns a short, stable name for the minimega instance that
// uses base: the first 6 hex characters of the SHA-256 of the clean absolute
// path.
func instanceName(base string) string {
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}

	sum := sha256.Sum256([]byte(filepath.Clean(base)))
	return hex.EncodeToString(sum[:])[:6]
}

// fcNetnsName returns the name of the network namespace for a Firecracker VM.
func fcNetnsName(id int) string {
	return fmt.Sprintf("mm-fc-%s-%d", fcInstance, id)
}

// fcParentCgroup returns the name of the parent cgroup for Firecracker VMs.
func fcParentCgroup() string {
	return "minimega-fc-" + fcInstance
}
