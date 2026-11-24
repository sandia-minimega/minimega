// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func setFCJailGlobals(t *testing.T, dir string) {
	t.Helper()

	oldDir, oldBase := *f_firecrackerDir, *f_firecrackerUIDBase
	oldInstance, oldVersion := fcInstance, hostCgroupVersion
	oldRoot, oldMounts, oldChown := fcCgroupRoot, fcMountinfoPath, fcChown
	oldMount, oldUnmount := fcMount, fcUnmount
	t.Cleanup(func() {
		*f_firecrackerDir, *f_firecrackerUIDBase = oldDir, oldBase
		fcInstance, hostCgroupVersion = oldInstance, oldVersion
		fcCgroupRoot, fcMountinfoPath, fcChown = oldRoot, oldMounts, oldChown
		fcMount, fcUnmount = oldMount, oldUnmount
	})

	*f_firecrackerDir = dir
	*f_firecrackerUIDBase = 200000
	fcInstance = "a1b2c3"
}

func TestFCJailerArgs(t *testing.T) {
	setFCJailGlobals(t, t.TempDir())

	j := fcJail{Base: "/fc/jail", ExecName: "firecracker", ID: "vm7", UID: 200007, GID: 200007}

	want := func(cgroup, version string, extra ...string) []string {
		args := []string{
			"--id", "vm7",
			"--exec-file", "/usr/bin/firecracker",
			"--uid", "200007",
			"--gid", "200007",
			"--cgroup-version", version,
			"--parent-cgroup", "minimega-fc-a1b2c3",
			"--cgroup", cgroup,
			"--chroot-base-dir", "/fc/jail",
			"--netns", "/proc/42/fd/7",
			"--",
			"--api-sock", "/api.sock",
		}
		return append(args, extra...)
	}

	for _, tc := range []struct {
		name    string
		version int
		pci     bool
		want    []string
	}{
		{"v2", 2, false, want("cpu.weight=100", "2")},
		{"v1", 1, false, want("cpu.shares=1024", "1")},
		{"v2 pci", 2, true, want("cpu.weight=100", "2", "--enable-pci")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := fcJailerArgs(j, "/usr/bin/firecracker", "/proc/42/fd/7", tc.version, tc.pci)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("fcJailerArgs() =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestNewFCJail(t *testing.T) {
	dir := t.TempDir()
	setFCJailGlobals(t, dir)

	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(bin, "firecracker-v1.17.0-x86_64")
	if err := os.WriteFile(target, nil, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bin, "firecracker")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	j := newFCJail(7, link)

	base := filepath.Join(fcResolve(dir), "jail")
	root := filepath.Join(base, "firecracker-v1.17.0-x86_64", "vm7", "root")
	want := fcJail{
		Base:     base,
		ExecName: "firecracker-v1.17.0-x86_64",
		ID:       "vm7",
		Root:     root,
		UID:      200007,
		GID:      200007,
	}
	if j.mounts == nil || len(*j.mounts) != 0 {
		t.Fatalf("newFCJail() mount record = %v, want an empty record", j.mounts)
	}
	want.mounts = j.mounts
	if j != want {
		t.Fatalf("newFCJail() = %+v, want %+v", j, want)
	}

	for _, tc := range [][2]string{
		{"/api.sock", filepath.Join(root, "api.sock")},
		{"rootfs.raw", filepath.Join(root, "rootfs.raw")},
		{"out/mem", filepath.Join(root, "out", "mem")},
		{"../../etc/passwd", filepath.Join(root, "etc", "passwd")},
		{"/out/../../v.sock", filepath.Join(root, "v.sock")},
	} {
		if got := j.HostPath(tc[0]); got != tc[1] {
			t.Errorf("HostPath(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}
}

func TestFCCheckSocketPath(t *testing.T) {
	ok := "/" + strings.Repeat("a", 106)
	if len(ok) != 107 {
		t.Fatalf("bad fixture length %d", len(ok))
	}
	if err := fcCheckSocketPath(ok); err != nil {
		t.Errorf("107-byte path: %v", err)
	}

	long := ok + "b"
	err := fcCheckSocketPath(long)
	if err == nil {
		t.Fatal("108-byte path: no error")
	}
	if !strings.Contains(err.Error(), long) || !strings.Contains(err.Error(), "-firecracker-dir") {
		t.Errorf("108-byte path: error %q does not name the path and -firecracker-dir", err)
	}
}

func TestFCMountsUnder(t *testing.T) {
	mountinfo := strings.Join([]string{
		`22 1 8:1 / / rw,relatime shared:1 - ext4 /dev/sda1 rw`,
		`30 22 8:1 /img/base.raw /fc/jail/firecracker/vm3/root/rootfs.raw rw,relatime shared:1 - ext4 /dev/sda1 rw`,
		`31 22 8:1 /img/a /fc/jail/firecracker/vm3/root/out/a\040b rw,relatime shared:1 - ext4 /dev/sda1 rw`,
		`32 22 8:1 /img/c /fc/jail/firecracker/vm30/root/rootfs.raw rw,relatime shared:1 - ext4 /dev/sda1 rw`,
		`33 22 0:40 / /fc/jail/firecracker/vm3 rw,relatime shared:2 - tmpfs tmpfs rw`,
		`34 22 8:1 /img/d /fc/jail/firecracker/vm4/root/rootfs.raw rw,relatime shared:1 - ext4 /dev/sda1 rw`,
		``,
	}, "\n")

	got := fcMountsUnder(mountinfo, "/fc/jail/firecracker/vm3/")
	want := []string{
		"/fc/jail/firecracker/vm3/root/out/a b",
		"/fc/jail/firecracker/vm3/root/rootfs.raw",
		"/fc/jail/firecracker/vm3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fcMountsUnder() = %q, want %q", got, want)
	}

	if got := fcUnescapeMount(`a\134b\011c\0`); got != "a\\b\tc\\0" {
		t.Errorf("fcUnescapeMount() = %q", got)
	}
}

func TestFCJailPrepareLinkDestroy(t *testing.T) {
	dir := t.TempDir()
	setFCJailGlobals(t, dir)

	mounts := filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(mounts, []byte("22 1 8:1 / / rw - ext4 /dev/sda1 rw\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fcMountinfoPath = mounts

	type chown struct {
		path     string
		uid, gid int
	}
	var chowns []chown
	fcChown = func(path string, uid, gid int) error {
		chowns = append(chowns, chown{path, uid, gid})
		return nil
	}

	for _, version := range []int{1, 2} {
		chowns = nil
		hostCgroupVersion = version
		fcCgroupRoot = filepath.Join(dir, "cgroup", strconv.Itoa(version))

		bin := filepath.Join(dir, "firecracker")
		if err := os.WriteFile(bin, nil, 0755); err != nil {
			t.Fatal(err)
		}
		j := newFCJail(3, bin)

		var cgroups []string
		if version == 2 {
			cgroups = []string{filepath.Join(fcCgroupRoot, fcParentCgroup(), "vm3")}
		} else {
			cgroups = []string{
				filepath.Join(fcCgroupRoot, "cpu,cpuacct", fcParentCgroup(), "vm3"),
				filepath.Join(fcCgroupRoot, "memory", fcParentCgroup(), "vm3"),
			}
			if err := os.MkdirAll(filepath.Join(fcCgroupRoot, "pids"), 0755); err != nil {
				t.Fatal(err)
			}
		}
		for _, cg := range cgroups {
			if err := os.MkdirAll(cg, 0755); err != nil {
				t.Fatal(err)
			}
		}

		// leftovers from an earlier launch
		if err := os.MkdirAll(filepath.Join(j.Root, "dev", "net"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(j.HostPath("api.sock"), nil, 0600); err != nil {
			t.Fatal(err)
		}

		if err := j.Prepare(); err != nil {
			t.Fatalf("v%d Prepare: %v", version, err)
		}

		if _, err := os.Stat(j.HostPath("api.sock")); !os.IsNotExist(err) {
			t.Errorf("v%d Prepare left api.sock: %v", version, err)
		}
		if _, err := os.Stat(j.HostPath("dev")); !os.IsNotExist(err) {
			t.Errorf("v%d Prepare left dev: %v", version, err)
		}
		for _, cg := range cgroups {
			if _, err := os.Stat(cg); !os.IsNotExist(err) {
				t.Errorf("v%d Prepare left cgroup %v: %v", version, cg, err)
			}
		}
		for _, d := range []string{j.Root, j.HostPath("out")} {
			fi, err := os.Stat(d)
			if err != nil {
				t.Fatalf("v%d: %v", version, err)
			}
			if !fi.IsDir() || fi.Mode().Perm() != 0700 {
				t.Errorf("v%d: %v mode %v, want 0700 directory", version, d, fi.Mode())
			}
		}
		wantChowns := []chown{{j.Root, 200003, 200003}, {j.HostPath("out"), 200003, 200003}}
		if !reflect.DeepEqual(chowns, wantChowns) {
			t.Errorf("v%d chowns = %v, want %v", version, chowns, wantChowns)
		}

		src := filepath.Join(dir, "disk.raw")
		if err := os.WriteFile(src, []byte("disk"), 0600); err != nil {
			t.Fatal(err)
		}

		for i := 0; i < 2; i++ {
			method, err := j.Link(src, "/rootfs.raw")
			if err != nil {
				t.Fatalf("v%d Link %d: %v", version, i, err)
			}
			if method != "hard link" {
				t.Errorf("v%d Link %d method = %q, want hard link", version, i, method)
			}
		}

		a, err := os.Stat(src)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.Stat(j.HostPath("rootfs.raw"))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(a, b) {
			t.Errorf("v%d rootfs.raw is not a hard link to %v", version, src)
		}

		chowns = nil
		if _, err := j.Link(src, "boot/vmlinux"); err != nil {
			t.Fatalf("v%d Link boot/vmlinux: %v", version, err)
		}
		if want := []chown{{j.HostPath("boot"), 200003, 200003}}; !reflect.DeepEqual(chowns, want) {
			t.Errorf("v%d nested Link chowns = %v, want %v", version, chowns, want)
		}

		if err := j.Destroy(); err != nil {
			t.Fatalf("v%d Destroy: %v", version, err)
		}
		if _, err := os.Stat(j.Dir()); !os.IsNotExist(err) {
			t.Errorf("v%d Destroy left %v: %v", version, j.Dir(), err)
		}
		if _, err := os.Stat(src); err != nil {
			t.Errorf("v%d Destroy removed the source: %v", version, err)
		}

		if err := j.Destroy(); err != nil {
			t.Errorf("v%d second Destroy: %v", version, err)
		}
	}
}

// A VMM with the shared group of a snapshot gets jail objects that only its
// uid can use.
func TestFCJailSharedGID(t *testing.T) {
	dir := t.TempDir()
	setFCJailGlobals(t, dir)

	fcMountinfoPath = filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(fcMountinfoPath, nil, 0644); err != nil {
		t.Fatal(err)
	}
	hostCgroupVersion = 2
	fcCgroupRoot = filepath.Join(dir, "cgroup")

	type chown struct {
		path     string
		uid, gid int
	}
	var chowns []chown
	fcChown = func(path string, uid, gid int) error {
		chowns = append(chowns, chown{path, uid, gid})
		return nil
	}

	bin := filepath.Join(dir, "firecracker")
	if err := os.WriteFile(bin, nil, 0755); err != nil {
		t.Fatal(err)
	}
	j := newFCJail(3, bin)
	j.GID = 199999

	if err := j.Prepare(); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "disk.raw")
	if err := os.WriteFile(src, []byte("disk"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Link(src, "boot/vmlinux"); err != nil {
		t.Fatal(err)
	}

	want := []chown{{j.Root, 200003, 200003}, {j.HostPath("out"), 200003, 200003}, {j.HostPath("boot"), 200003, 200003}}
	if !reflect.DeepEqual(chowns, want) {
		t.Errorf("chowns = %v, want %v", chowns, want)
	}
	for _, d := range []string{j.Root, j.HostPath("out"), j.HostPath("boot")} {
		fi, err := os.Stat(d)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0700 {
			t.Errorf("%v mode = %v, want 0700", d, fi.Mode().Perm())
		}
	}

	args := fcJailerArgs(j, bin, "/proc/42/fd/7", 2, false)
	if fcArg(args, "--uid") != "200003" || fcArg(args, "--gid") != "199999" {
		t.Errorf("jailer args = %q, want --uid 200003 --gid 199999", args)
	}

	if err := j.Destroy(); err != nil {
		t.Fatal(err)
	}
}

func TestFCJailLinkEXDEV(t *testing.T) {
	dir := t.TempDir()
	setFCJailGlobals(t, dir)

	oldLink, oldMount := fcHardLink, fcBindMount
	t.Cleanup(func() { fcHardLink, fcBindMount = oldLink, oldMount })

	fcMountinfoPath = filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(fcMountinfoPath, []byte("22 1 8:1 / / rw - ext4 /dev/sda1 rw\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fcChown = func(string, int, int) error { return nil }
	hostCgroupVersion = 2
	fcCgroupRoot = filepath.Join(dir, "cgroup")

	j := newFCJail(3, filepath.Join(dir, "firecracker"))
	if err := j.Prepare(); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, "disk.raw")
	if err := os.WriteFile(src, []byte("disk"), 0600); err != nil {
		t.Fatal(err)
	}
	dst := j.HostPath("rootfs.raw")

	var mounts [][2]string
	fcBindMount = func(s, d string) error {
		mounts = append(mounts, [2]string{s, d})
		return nil
	}

	fcHardLink = func(oldname, newname string) error {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: unix.EXDEV}
	}
	method, err := j.Link(src, "/rootfs.raw")
	if err != nil {
		t.Fatalf("Link with EXDEV: %v", err)
	}
	if method != "bind mount" {
		t.Errorf("Link with EXDEV method = %q, want bind mount", method)
	}
	if want := [][2]string{{src, dst}}; !reflect.DeepEqual(mounts, want) {
		t.Errorf("bind mounts = %v, want %v", mounts, want)
	}
	if fi, err := os.Stat(dst); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("bind mount target %v: %v", dst, err)
	}

	mounts = nil
	fcHardLink = func(oldname, newname string) error {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: unix.EPERM}
	}
	if _, err := j.Link(src, "/rootfs.raw"); err == nil || !strings.Contains(err.Error(), "hard link") {
		t.Errorf("Link with EPERM = %v, want a hard link error", err)
	}
	if len(mounts) != 0 {
		t.Errorf("Link with EPERM made bind mounts %v", mounts)
	}
}

func TestFCJailTrackedMounts(t *testing.T) {
	dir := t.TempDir()
	setFCJailGlobals(t, dir)

	// A read of the mount table fails.
	fcMountinfoPath = filepath.Join(dir, "no-mountinfo")
	fcChown = func(string, int, int) error { return nil }
	hostCgroupVersion = 2
	fcCgroupRoot = filepath.Join(dir, "cgroup")

	var mounted, unmounted []string
	fcMount = func(src, dst, fstype string, flags uintptr, data string) error {
		mounted = append(mounted, dst)
		return nil
	}
	fcUnmount = func(target string, flags int) error {
		unmounted = append(unmounted, target)
		return nil
	}

	bin := filepath.Join(dir, "firecracker")
	if err := os.WriteFile(bin, nil, 0755); err != nil {
		t.Fatal(err)
	}
	j := newFCJail(3, bin)

	if err := j.Prepare(); err != nil {
		t.Fatalf("Prepare of a new jail: %v", err)
	}

	// /dev/null is on another filesystem, so Link bind mounts it.
	for _, name := range []string{"/a", "sub/b", "/c"} {
		method, err := j.Link(os.DevNull, name)
		if err != nil {
			t.Fatalf("Link %v: %v", name, err)
		}
		if method != "bind mount" {
			t.Skipf("Link %v used %v: %v and the jail are on one filesystem", name, method, os.DevNull)
		}
	}
	want := []string{j.HostPath("a"), j.HostPath("sub/b"), j.HostPath("c")}
	if !reflect.DeepEqual(mounted, want) {
		t.Errorf("mounts = %q, want %q", mounted, want)
	}

	// A copy of the jail shares the record.
	k := j
	if err := k.Destroy(); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if want := []string{j.HostPath("sub/b"), j.HostPath("c"), j.HostPath("a")}; !reflect.DeepEqual(unmounted, want) {
		t.Errorf("unmounts = %q, want %q", unmounted, want)
	}
	if _, err := os.Stat(j.Dir()); !os.IsNotExist(err) {
		t.Errorf("Destroy left %v: %v", j.Dir(), err)
	}

	unmounted = nil
	if err := j.Destroy(); err != nil || len(unmounted) != 0 {
		t.Errorf("second Destroy = %v, unmounts %q", err, unmounted)
	}

	// A jail without a record, or a jail directory that is left, needs the
	// mount table.
	untracked := j
	untracked.mounts = nil
	if err := untracked.Destroy(); err == nil || !strings.Contains(err.Error(), "read mounts") {
		t.Errorf("Destroy without a record = %v, want a mount table read error", err)
	}
	if err := os.MkdirAll(j.Root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := j.Prepare(); err == nil || !strings.Contains(err.Error(), "read mounts") {
		t.Errorf("Prepare over a left jail = %v, want a mount table read error", err)
	}
}

func TestFCDeepestFirst(t *testing.T) {
	got := fcDeepestFirst([]string{"/j/a", "/j/x/b", "/j/a", "/j/c"})
	if want := []string{"/j/x/b", "/j/c", "/j/a", "/j/a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("fcDeepestFirst() = %q, want %q", got, want)
	}
}

func TestFCSetupParentCgroup(t *testing.T) {
	root := t.TempDir()
	parent := "minimega-fc-a1b2c3"

	if err := fcSetupParentCgroup(root, parent, 1); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, parent)); !os.IsNotExist(err) {
		t.Errorf("v1 created the parent cgroup: %v", err)
	}

	rootCtl := filepath.Join(root, "cgroup.subtree_control")
	if err := os.WriteFile(rootCtl, []byte("memory io\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := fcSetupParentCgroup(root, parent, 2); err == nil {
		t.Error("v2 without the parent's cgroup.subtree_control: no error")
	}

	parentCtl := filepath.Join(root, parent, "cgroup.subtree_control")
	if err := os.WriteFile(rootCtl, []byte("memory io\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parentCtl, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := fcSetupParentCgroup(root, parent, 2); err != nil {
		t.Fatalf("v2: %v", err)
	}
	for _, p := range []string{rootCtl, parentCtl} {
		if data, _ := os.ReadFile(p); string(data) != "+cpu" {
			t.Errorf("%v = %q, want +cpu", p, data)
		}
	}

	for _, p := range []string{rootCtl, parentCtl} {
		if err := os.WriteFile(p, []byte("cpu memory\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := fcSetupParentCgroup(root, parent, 2); err != nil {
		t.Fatalf("v2 enabled: %v", err)
	}
	for _, p := range []string{rootCtl, parentCtl} {
		if data, _ := os.ReadFile(p); string(data) != "cpu memory\n" {
			t.Errorf("%v = %q, want it unchanged", p, data)
		}
	}
}

func TestFCEnsureParentCgroupRetry(t *testing.T) {
	oldRoot, oldVersion := fcCgroupRoot, hostCgroupVersion
	t.Cleanup(func() {
		fcCgroupRoot, hostCgroupVersion = oldRoot, oldVersion
		fcCgroupMu.Lock()
		fcCgroupDone = false
		fcCgroupMu.Unlock()
	})

	fcCgroupMu.Lock()
	fcCgroupDone = false
	fcCgroupMu.Unlock()

	fcCgroupRoot = t.TempDir()
	hostCgroupVersion = 2

	if err := fcEnsureParentCgroup(); err == nil {
		t.Fatal("no cgroup.subtree_control: no error")
	}

	rootCtl := filepath.Join(fcCgroupRoot, "cgroup.subtree_control")
	parentCtl := filepath.Join(fcCgroupRoot, fcParentCgroup(), "cgroup.subtree_control")
	for _, p := range []string{rootCtl, parentCtl} {
		if err := os.WriteFile(p, []byte("cpu\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := fcEnsureParentCgroup(); err != nil {
		t.Fatalf("after the fix: %v", err)
	}
}
