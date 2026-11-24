// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/sys/unix"
)

// fcMaxSocketPath is the size of sun_path in struct sockaddr_un.
const fcMaxSocketPath = 108

// Tests replace these.
var (
	fcCgroupRoot    = hostCgroupRoot
	fcMountinfoPath = "/proc/self/mountinfo"
	fcChown         = os.Chown
	fcHardLink      = os.Link
	fcMount         = unix.Mount
	fcUnmount       = unix.Unmount
	fcBindMount     = func(src, dst string) error {
		return fcMount(src, dst, "", unix.MS_BIND, "")
	}
)

var (
	fcCgroupMu   sync.Mutex
	fcCgroupDone bool
)

// fcJail describes the jailer chroot for one Firecracker VM. The jailer
// builds it at <Base>/<ExecName>/<ID>/root.
type fcJail struct {
	Base     string
	ExecName string
	ID       string
	Root     string

	// UID is the uid of the VMM, and the owner and group of the jail objects
	// that minimega creates. GID is the gid of the VMM: UID, or the shared
	// group of the snapshot that the VMM loads.
	UID, GID int

	// mounts records the bind mounts that Link made, and copies of the jail
	// share it. It is nil for a jail that newFCJail did not return, and
	// Destroy then reads the mount table.
	mounts *[]string
}

// newFCJail returns the jail for VM vmID that runs the Firecracker binary
// fcBin.
func newFCJail(vmID int, fcBin string) fcJail {
	j := fcJail{
		Base:     fcJailBase(),
		ExecName: filepath.Base(fcResolve(fcBin)),
		ID:       fmt.Sprintf("vm%d", vmID),
		UID:      fcVMUID(vmID),
		mounts:   new([]string),
	}
	j.GID = j.UID
	j.Root = filepath.Join(j.Dir(), "root")

	return j
}

// fcJailBase returns the jailer --chroot-base-dir.
func fcJailBase() string {
	return filepath.Join(fcResolve(*f_firecrackerDir), "jail")
}

// fcVMUID returns the uid of the VMM of VM vmID. It is also the gid of a VMM
// that does not load a snapshot.
func fcVMUID(vmID int) int {
	return int(*f_firecrackerUIDBase) + vmID
}

// Dir returns the per-VM jail directory that holds Root.
func (j fcJail) Dir() string {
	return filepath.Join(j.Base, j.ExecName, j.ID)
}

// HostPath returns the host path of a path inside the jail.
func (j fcJail) HostPath(inJail string) string {
	return filepath.Join(j.Root, filepath.Clean("/"+inJail))
}

// fcCheckSocketPath returns an error when p is too long for a Unix socket.
func fcCheckSocketPath(p string) error {
	if len(p) >= fcMaxSocketPath {
		return fmt.Errorf("socket path %v is %d bytes, but a Unix socket path must be shorter than %d bytes: use a shorter -firecracker-dir", p, len(p), fcMaxSocketPath)
	}

	return nil
}

// fcJailerArgs returns the jailer arguments that start Firecracker in API
// mode. The jailer execs Firecracker in place, so the jailer PID is the VMM.
func fcJailerArgs(j fcJail, fcBin, netnsPath string, cgroupVersion int, enablePCI bool) []string {
	cgroup := "cpu.weight=100"
	if cgroupVersion != 2 {
		cgroup = "cpu.shares=1024"
	}

	args := []string{
		"--id", j.ID,
		"--exec-file", fcBin,
		"--uid", strconv.Itoa(j.UID),
		"--gid", strconv.Itoa(j.GID),
		"--cgroup-version", strconv.Itoa(cgroupVersion),
		"--parent-cgroup", fcParentCgroup(),
		"--cgroup", cgroup,
		"--chroot-base-dir", j.Base,
		"--netns", netnsPath,
		"--",
		"--api-sock", "/api.sock",
	}

	if enablePCI {
		args = append(args, "--enable-pci")
	}

	return args
}

// fcEnsureParentCgroup creates the parent cgroup for Firecracker VMs. After a
// success it does nothing. After a failure the next call tries again.
func fcEnsureParentCgroup() error {
	fcCgroupMu.Lock()
	defer fcCgroupMu.Unlock()

	if fcCgroupDone {
		return nil
	}
	if err := fcSetupParentCgroup(fcCgroupRoot, fcParentCgroup(), hostCgroupVersion); err != nil {
		return err
	}
	fcCgroupDone = true

	return nil
}

// fcSetupParentCgroup creates root/parent and enables the cpu controller for
// its children. The parent holds no processes. cgroup v1 needs no setup.
func fcSetupParentCgroup(root, parent string, version int) error {
	if version != 2 {
		return nil
	}

	dir := filepath.Join(root, parent)
	if err := os.Mkdir(dir, 0755); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create parent cgroup: %v", err)
	}

	for _, d := range []string{root, dir} {
		if err := fcEnableController(filepath.Join(d, "cgroup.subtree_control"), "cpu"); err != nil {
			return err
		}
	}

	return nil
}

func fcEnableController(path, ctrl string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %v: %v", path, err)
	}

	for _, c := range strings.Fields(string(data)) {
		if c == ctrl {
			return nil
		}
	}

	if err := os.WriteFile(path, []byte("+"+ctrl), 0644); err != nil {
		if errors.Is(err, unix.EBUSY) {
			return fmt.Errorf("enable %v controller in %v: %v: the cgroup holds processes, so move them into a child cgroup first (docker/start-minimega.sh does this in a container)", ctrl, path, err)
		}
		return fmt.Errorf("enable %v controller in %v: %v", ctrl, path, err)
	}

	return nil
}

// Prepare removes any jail left from an earlier launch and creates an empty
// jail root and /out, owned by the VM uid with mode 0700. It reads the mount
// table only when a jail directory is left.
func (j fcJail) Prepare() error {
	_, statErr := os.Lstat(j.Dir())
	if err := j.destroy(j.mounts == nil || statErr == nil); err != nil {
		return err
	}

	for _, d := range []string{j.Root, j.HostPath("out")} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return fmt.Errorf("create jail: %v", err)
		}
		if err := fcChown(d, j.UID, j.UID); err != nil {
			return fmt.Errorf("create jail: %v", err)
		}
	}

	return nil
}

// Link makes src available at name inside the jail. A block device gets a new
// device node, a file on the jail filesystem gets a hard link, and any other
// file gets a bind mount. A hard link that fails with EXDEV, for example
// between two mounts of one filesystem, also gets a bind mount. It returns
// the method it used.
func (j fcJail) Link(src, name string) (string, error) {
	dst := j.HostPath(name)

	if err := j.mkdirs(filepath.Dir(dst)); err != nil {
		return "", err
	}

	var st, root unix.Stat_t
	if err := unix.Stat(src, &st); err != nil {
		return "", fmt.Errorf("link %v into jail: %v", src, err)
	}
	if err := unix.Stat(j.Root, &root); err != nil {
		return "", fmt.Errorf("link %v into jail: %v", src, err)
	}

	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove stale %v: %v", dst, err)
	}

	if st.Mode&unix.S_IFMT == unix.S_IFBLK {
		if err := unix.Mknod(dst, unix.S_IFBLK|0600, int(st.Rdev)); err != nil {
			return "", fmt.Errorf("create device node %v: %v", dst, err)
		}
		if err := fcChown(dst, j.UID, j.UID); err != nil {
			return "", fmt.Errorf("create device node %v: %v", dst, err)
		}
		return "device node", nil
	}

	if st.Dev == root.Dev {
		err := fcHardLink(src, dst)
		if err == nil {
			return "hard link", nil
		}
		if !errors.Is(err, unix.EXDEV) {
			return "", fmt.Errorf("hard link %v to %v: %v", src, dst, err)
		}
		log.Debug("hard link %v to %v: %v; using a bind mount", src, dst, err)
	}

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("create bind mount target %v: %v", dst, err)
	}
	f.Close()

	if err := fcBindMount(src, dst); err != nil {
		return "", fmt.Errorf("bind mount %v to %v: %v", src, dst, err)
	}
	if j.mounts != nil {
		*j.mounts = append(*j.mounts, dst)
	}

	return "bind mount", nil
}

// mkdirs creates dir and its missing parents inside Root, owned by the VM
// uid.
func (j fcJail) mkdirs(dir string) error {
	var missing []string
	for d := dir; d != j.Root && strings.HasPrefix(d, j.Root+"/"); d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			break
		}
		missing = append(missing, d)
	}

	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0700); err != nil && !os.IsExist(err) {
			return fmt.Errorf("create %v: %v", missing[i], err)
		}
		if err := fcChown(missing[i], j.UID, j.UID); err != nil {
			return fmt.Errorf("create %v: %v", missing[i], err)
		}
	}

	return nil
}

// Destroy unmounts the bind mounts that Link made, removes the jail
// directory, and removes the VM cgroup. Missing paths are not errors. It reads
// the mount table when the jail has no mount record or when the directory
// stays busy.
func (j fcJail) Destroy() error {
	return j.destroy(j.mounts == nil)
}

func (j fcJail) destroy(scan bool) error {
	dir := j.Dir()

	if scan {
		if err := fcUnmountUnder(dir); err != nil {
			return err
		}
	} else if err := fcUnmountAll(fcDeepestFirst(*j.mounts)); err != nil {
		return err
	}
	if j.mounts != nil {
		*j.mounts = nil
	}

	if err := os.RemoveAll(dir); err != nil {
		if scan {
			return fmt.Errorf("remove jail: %v", err)
		}
		if err := fcUnmountUnder(dir); err != nil {
			return err
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove jail: %v", err)
		}
	}

	for _, cg := range fcVMCgroups(fcCgroupRoot, fcParentCgroup(), j.ID, hostCgroupVersion) {
		if err := fcRmdirRetry(cg); err != nil {
			return fmt.Errorf("remove cgroup %v: %v", cg, err)
		}
	}

	return nil
}

// fcVMCgroups returns the cgroup directories that the jailer may create for
// VM id. On cgroup v1, there is one for each controller hierarchy.
func fcVMCgroups(root, parent, id string, version int) []string {
	if version == 2 {
		return []string{filepath.Join(root, parent, id)}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var res []string
	for _, e := range entries {
		res = append(res, filepath.Join(root, e.Name(), parent, id))
	}

	return res
}

func fcRmdirRetry(dir string) error {
	for i := 0; ; i++ {
		err := unix.Rmdir(dir)
		if err == nil || err == unix.ENOENT || err == unix.ENOTDIR {
			return nil
		}
		if err != unix.EBUSY || i == 19 {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// fcUnmountUnder unmounts every mount point in the mount table that is dir or
// below it.
func fcUnmountUnder(dir string) error {
	data, err := os.ReadFile(fcMountinfoPath)
	if err != nil {
		return fmt.Errorf("read mounts: %v", err)
	}

	return fcUnmountAll(fcMountsUnder(string(data), dir))
}

// fcUnmountAll detaches each mount point in order. A path that is gone or not
// a mount point is not an error.
func fcUnmountAll(mounts []string) error {
	for _, m := range mounts {
		if err := fcUnmount(m, unix.MNT_DETACH); err != nil && err != unix.EINVAL && err != unix.ENOENT {
			return fmt.Errorf("unmount %v: %v", m, err)
		}
	}

	return nil
}

// fcMountsUnder returns the mount points in mountinfo that are dir or below
// it, deepest first.
func fcMountsUnder(mountinfo, dir string) []string {
	dir = filepath.Clean(dir)

	var res []string
	for _, line := range strings.Split(mountinfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		mp := fcUnescapeMount(fields[4])
		if mp == dir || strings.HasPrefix(mp, dir+"/") {
			res = append(res, mp)
		}
	}

	return fcDeepestFirst(res)
}

// fcDeepestFirst sorts paths by depth, deepest first, and keeps the later
// mount first among paths of the same depth.
func fcDeepestFirst(paths []string) []string {
	res := make([]string, len(paths))
	for i, p := range paths {
		res[len(paths)-1-i] = p
	}

	sort.SliceStable(res, func(a, b int) bool {
		return strings.Count(res[a], "/") > strings.Count(res[b], "/")
	})

	return res
}

// fcUnescapeMount decodes the octal escapes, such as \040, in a mountinfo
// field.
func fcUnescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}

	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}

	return b.String()
}
