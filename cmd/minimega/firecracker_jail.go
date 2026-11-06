// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// fcMaxSocketPath is the size of sun_path in struct sockaddr_un.
const fcMaxSocketPath = 108

// Tests replace these.
var (
	fcCgroupRoot    = cgroupRoot
	fcMountinfoPath = "/proc/self/mountinfo"
	fcChown         = os.Chown
)

var (
	fcCgroupOnce sync.Once
	fcCgroupErr  error
)

// fcJail describes the jailer chroot for one Firecracker VM. The jailer
// builds it at <Base>/<ExecName>/<ID>/root.
type fcJail struct {
	Base     string
	ExecName string
	ID       string
	Root     string
	UID, GID int
}

// newFCJail returns the jail for VM vmID that runs the Firecracker binary
// fcBin.
func newFCJail(vmID int, fcBin string) fcJail {
	j := fcJail{
		Base:     filepath.Join(fcResolve(*f_firecrackerDir), "jail"),
		ExecName: filepath.Base(fcResolve(fcBin)),
		ID:       fmt.Sprintf("vm%d", vmID),
		UID:      int(*f_firecrackerUIDBase) + vmID,
	}
	j.GID = j.UID
	j.Root = filepath.Join(j.Dir(), "root")

	return j
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

// fcEnsureParentCgroup creates the parent cgroup for Firecracker VMs once per
// process and caches the result.
func fcEnsureParentCgroup() error {
	fcCgroupOnce.Do(func() {
		fcCgroupErr = fcSetupParentCgroup(fcCgroupRoot, fcParentCgroup(), hostCgroupVersion)
	})

	return fcCgroupErr
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
		return fmt.Errorf("enable %v controller in %v: %v", ctrl, path, err)
	}

	return nil
}

// Prepare removes any jail left from an earlier launch and creates an empty
// jail root and /out, owned by the VM uid.
func (j fcJail) Prepare() error {
	if err := j.Destroy(); err != nil {
		return err
	}

	for _, d := range []string{j.Root, j.HostPath("out")} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return fmt.Errorf("create jail: %v", err)
		}
		if err := fcChown(d, j.UID, j.GID); err != nil {
			return fmt.Errorf("create jail: %v", err)
		}
	}

	return nil
}

// Link makes src available at name inside the jail. A block device gets a new
// device node, a file on the jail filesystem gets a hard link, and any other
// file gets a bind mount. It returns the method it used.
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
		if err := fcChown(dst, j.UID, j.GID); err != nil {
			return "", fmt.Errorf("create device node %v: %v", dst, err)
		}
		return "device node", nil
	}

	if st.Dev == root.Dev {
		if err := os.Link(src, dst); err != nil {
			return "", fmt.Errorf("hard link %v to %v: %v", src, dst, err)
		}
		return "hard link", nil
	}

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("create bind mount target %v: %v", dst, err)
	}
	f.Close()

	if err := unix.Mount(src, dst, "", unix.MS_BIND, ""); err != nil {
		return "", fmt.Errorf("bind mount %v to %v: %v", src, dst, err)
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
		if err := fcChown(missing[i], j.UID, j.GID); err != nil {
			return fmt.Errorf("create %v: %v", missing[i], err)
		}
	}

	return nil
}

// Destroy unmounts everything under the jail directory, removes it, and
// removes the VM cgroup. Missing paths are not errors.
func (j fcJail) Destroy() error {
	dir := j.Dir()

	data, err := os.ReadFile(fcMountinfoPath)
	if err != nil {
		return fmt.Errorf("read mounts: %v", err)
	}

	for _, m := range fcMountsUnder(string(data), dir) {
		if err := unix.Unmount(m, unix.MNT_DETACH); err != nil && err != unix.EINVAL && err != unix.ENOENT {
			return fmt.Errorf("unmount %v: %v", m, err)
		}
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove jail: %v", err)
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
