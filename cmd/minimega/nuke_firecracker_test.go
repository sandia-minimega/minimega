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
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

type nukeOps struct {
	mu  sync.Mutex
	ops []string
}

func (o *nukeOps) add(s string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ops = append(o.ops, s)
}

func (o *nukeOps) get() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.ops...)
}

func nukeWrite(t *testing.T, path, data string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func nukeMkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

// fakePID returns a fake PID that is not the PID of the test.
func fakePID(n int) string {
	return strconv.Itoa(4100000 + n)
}

// nukeProc writes a fake proc directory for fakePID(n).
func nukeProc(t *testing.T, root string, n int, cgroup string, args ...string) {
	t.Helper()

	pid := 4100000 + n
	dir := filepath.Join(root, strconv.Itoa(pid))
	nukeWrite(t, filepath.Join(dir, "stat"), strconv.Itoa(pid)+" (x) S 1 1 1 0\n")
	nukeWrite(t, filepath.Join(dir, "cgroup"), cgroup+"\n")
	if len(args) > 0 {
		nukeWrite(t, filepath.Join(dir, "cmdline"), strings.Join(args, "\x00")+"\x00")
	}
}

func mountLine(mp string) string {
	return "36 35 98:0 / " + mp + " rw,noatime master:1 - ext4 /dev/root rw\n"
}

// setNukeSeams replaces the nuke seams with fakes that record their calls in
// ops. The fake unmount removes the mount from the mountinfo file, except for
// the mounts in busy.
func setNukeSeams(t *testing.T, root string, ops *nukeOps, dmls string, busy map[string]bool) {
	t.Helper()

	oldProc, oldBlock, oldNetns := nukeProcRoot, nukeSysBlock, nukeNetnsDir
	oldRun, oldKill, oldUnmount, oldDestroy := nukeRunCmd, nukeKill, nukeUnmount, nukeRemoveNetns
	t.Cleanup(func() {
		nukeProcRoot, nukeSysBlock, nukeNetnsDir = oldProc, oldBlock, oldNetns
		nukeRunCmd, nukeKill, nukeUnmount, nukeRemoveNetns = oldRun, oldKill, oldUnmount, oldDestroy
	})

	nukeProcRoot = filepath.Join(root, "proc")
	nukeSysBlock = filepath.Join(root, "sys", "block")
	nukeNetnsDir = filepath.Join(root, "netns")
	fcCgroupRoot = filepath.Join(root, "cgroup")
	fcMountinfoPath = filepath.Join(root, "mountinfo")
	hostCgroupVersion = 2

	nukeKill = func(pid int) error {
		ops.add("kill " + strconv.Itoa(pid))
		return os.RemoveAll(filepath.Join(nukeProcRoot, strconv.Itoa(pid)))
	}
	nukeUnmount = func(p string) error {
		ops.add("umount " + p)
		if busy[p] {
			return unix.EBUSY
		}

		data, err := os.ReadFile(fcMountinfoPath)
		if err != nil {
			return err
		}
		var keep []string
		for _, line := range strings.SplitAfter(string(data), "\n") {
			if f := strings.Fields(line); len(f) > 4 && f[4] == p {
				continue
			}
			keep = append(keep, line)
		}
		return os.WriteFile(fcMountinfoPath, []byte(strings.Join(keep, "")), 0644)
	}
	nukeRunCmd = func(args ...string) (string, error) {
		ops.add("run " + strings.Join(args, " "))
		if reflect.DeepEqual(args, []string{"dmsetup", "ls"}) {
			return dmls, nil
		}
		return "", nil
	}
	nukeRemoveNetns = func(_, ns string) error {
		ops.add("netns " + ns)
		return nil
	}
}

func TestFCNuke(t *testing.T) {
	root := fcResolve(t.TempDir())
	dir := filepath.Join(root, "fc")
	setFCJailGlobals(t, dir)

	jail := filepath.Join(dir, "jail")
	vmRoot := filepath.Join(jail, "firecracker", "vm3", "root")
	proc := filepath.Join(root, "proc")

	// Processes of this instance: in the parent cgroup (v2 and v1), a jailer
	// before it joins the cgroup, and a VMM run from the jail.
	nukeProc(t, proc, 100, "0::/minimega-fc-a1b2c3/vm3", "/firecracker", "--id", "vm3")
	nukeProc(t, proc, 101, "0::/user.slice", "jailer", "--id", "vm4", "--parent-cgroup", "minimega-fc-a1b2c3")
	nukeProc(t, proc, 102, "0::/", "jailer", "--chroot-base-dir", jail+"/", "--id", "vm5")
	nukeProc(t, proc, 103, "0::/")
	if err := os.Symlink(filepath.Join(jail, "firecracker", "vm6", "root", "firecracker"), filepath.Join(proc, fakePID(103), "exe")); err != nil {
		t.Fatal(err)
	}
	nukeProc(t, proc, 104, "4:cpu,cpuacct:/minimega-fc-a1b2c3/vm7")

	// Processes of other instances and other programs.
	nukeProc(t, proc, 200, "0::/minimega-fc-ffffff/vm3", "jailer", "--parent-cgroup", "minimega-fc-ffffff", "--chroot-base-dir", "/other/jail")
	nukeProc(t, proc, 201, "0::/system.slice", "qemu-system-x86_64", "-name", "3")
	nukeProc(t, proc, 202, "0::/minimega-fc-a1b2c3x/vm1")
	nukeMkdir(t, filepath.Join(proc, "self"))

	nukeWrite(t, filepath.Join(root, "mountinfo"), mountLine(dir)+mountLine(jail)+
		mountLine(filepath.Join(vmRoot, "disk0"))+
		mountLine(filepath.Join(vmRoot, "vmlinux"))+
		mountLine(filepath.Join(vmRoot, "a", "b", "deep"))+
		mountLine(filepath.Join(dir, "vm", "9", "stuck"))+
		mountLine("/other/mount"))

	block := filepath.Join(root, "sys", "block")
	nukeWrite(t, filepath.Join(block, "loop0", "loop", "backing_file"), filepath.Join(dir, "vm", "3", "disk0.cow")+"\n")
	nukeWrite(t, filepath.Join(block, "loop1", "loop", "backing_file"), filepath.Join(dir, "cache", "abc")+" (deleted)\n")
	nukeWrite(t, filepath.Join(block, "loop2", "loop", "backing_file"), "/srv/other.img\n")
	nukeMkdir(t, filepath.Join(block, "loop3"))
	nukeMkdir(t, filepath.Join(block, "sda"))

	for _, n := range []string{"mm-fc-a1b2c3-3", "mm-fc-a1b2c3-10", "mm-fc-ffffff-3", "other"} {
		nukeWrite(t, filepath.Join(root, "netns", n), "")
	}

	cg := filepath.Join(root, "cgroup")
	for _, d := range []string{"minimega-fc-a1b2c3/vm3", "minimega-fc-a1b2c3/vm4", "minimega-fc-ffffff/vm1", "other"} {
		nukeMkdir(t, filepath.Join(cg, d))
	}

	nukeWrite(t, filepath.Join(vmRoot, "disk0"), "")
	nukeWrite(t, filepath.Join(dir, "vm", "3", "console.log"), "boot")
	nukeWrite(t, filepath.Join(dir, "vm", "9", "stuck"), "")
	nukeWrite(t, filepath.Join(dir, "cache", "abc"), "image")

	ops := &nukeOps{}
	dmls := "mm-fc-a1b2c3-3-0\t(253:0)\nmm-fc-a1b2c3-3-x1\t(253:1)\nmm-fc-ffffff-1-0\t(253:2)\nother\t(253:3)\n"
	busy := map[string]bool{filepath.Join(dir, "vm", "9", "stuck"): true}
	setNukeSeams(t, root, ops, dmls, busy)

	// The busy mount and the vm/9 entry under it stay.
	if n := fcNuke(); n != 19 {
		t.Errorf("fcNuke() = %v, want 19", n)
	}

	want := []string{
		"kill " + fakePID(100), "kill " + fakePID(101), "kill " + fakePID(102), "kill " + fakePID(103), "kill " + fakePID(104),
		"umount " + filepath.Join(vmRoot, "a", "b", "deep"),
		"umount " + filepath.Join(vmRoot, "vmlinux"),
		"umount " + filepath.Join(vmRoot, "disk0"),
		"umount " + filepath.Join(dir, "vm", "9", "stuck"),
		"run dmsetup ls",
		"run dmsetup remove mm-fc-a1b2c3-3-0",
		"run dmsetup remove mm-fc-a1b2c3-3-x1",
		"run losetup -d /dev/loop0",
		"run losetup -d /dev/loop1",
		"netns mm-fc-a1b2c3-10",
		"netns mm-fc-a1b2c3-3",
	}
	if got := ops.get(); !reflect.DeepEqual(got, want) {
		t.Errorf("operations =\n%q\nwant\n%q", got, want)
	}

	for _, p := range []string{
		filepath.Join(jail, "firecracker"),
		filepath.Join(dir, "vm", "3"),
		filepath.Join(cg, "minimega-fc-a1b2c3"),
	} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%v exists after nuke: %v", p, err)
		}
	}

	for _, p := range []string{
		jail,
		filepath.Join(dir, "vm", "9", "stuck"),
		filepath.Join(dir, "cache", "abc"),
		filepath.Join(cg, "minimega-fc-ffffff", "vm1"),
		filepath.Join(cg, "other"),
		filepath.Join(proc, fakePID(200)),
	} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%v is missing after nuke: %v", p, err)
		}
	}
}

func TestFCStartupClean(t *testing.T) {
	setup := func(t *testing.T) (root, dir, base string, ops *nukeOps) {
		root = fcResolve(t.TempDir())
		dir = filepath.Join(root, "fc")
		base = filepath.Join(root, "base")
		setFCJailGlobals(t, dir)

		oldBase := *f_base
		t.Cleanup(func() { *f_base = oldBase })
		*f_base = base

		// The <ID> links of the earlier process, a KVM instance directory, a
		// link into another Firecracker directory, and other base entries.
		nukeMkdir(t, base)
		for name, target := range map[string]string{
			"0":     filepath.Join(dir, "vm", "0"),
			"1":     filepath.Join(dir, "vm", "1"),
			"3":     filepath.Join(root, "other", "vm", "3"),
			"alias": filepath.Join(dir, "vm", "0"),
		} {
			if err := os.Symlink(target, filepath.Join(base, name)); err != nil {
				t.Fatal(err)
			}
		}
		nukeWrite(t, filepath.Join(base, "2", "name"), "kvm")
		nukeWrite(t, filepath.Join(base, "minimega.pid"), "1")
		nukeMkdir(t, filepath.Join(base, "files"))
		nukeMkdir(t, filepath.Join(base, "namespaces", "minimega"))

		// The namespace links of the earlier process: two to Firecracker VMs,
		// one to the KVM VM, and one to the link into the other directory.
		for name, target := range map[string]string{
			"live/fc0":  filepath.Join(base, "0"),
			"live/kvm2": filepath.Join(base, "2"),
			"old/fc1":   filepath.Join(base, "1"),
			"live/fc3":  filepath.Join(base, "3"),
		} {
			nukeMkdir(t, filepath.Join(base, "namespaces", filepath.Dir(name)))
			if err := os.Symlink(target, filepath.Join(base, "namespaces", name)); err != nil {
				t.Fatal(err)
			}
		}

		nukeWrite(t, filepath.Join(root, "mountinfo"), mountLine("/other/mount"))
		nukeMkdir(t, filepath.Join(root, "proc"))
		nukeMkdir(t, filepath.Join(root, "netns"))

		ops = &nukeOps{}
		setNukeSeams(t, root, ops, "mm-fc-a1b2c3-1-0\t(253:1)\nmm-fc-ffffff-1-0\t(253:2)\n", nil)

		return root, dir, base, ops
	}

	t.Run("leftovers", func(t *testing.T) {
		root, dir, base, ops := setup(t)

		vmRoot := filepath.Join(dir, "jail", "firecracker", "vm1", "root")
		nukeWrite(t, filepath.Join(vmRoot, "disk0"), "")
		nukeWrite(t, filepath.Join(dir, "vm", "0", "console.log"), "boot")
		nukeWrite(t, filepath.Join(dir, "cache", "abc"), "image")
		nukeWrite(t, filepath.Join(root, "mountinfo"), mountLine(filepath.Join(vmRoot, "disk0"))+mountLine("/other/mount"))

		nukeProc(t, filepath.Join(root, "proc"), 1, "0::/minimega-fc-a1b2c3/vm1", "/firecracker", "--id", "vm1")
		nukeProc(t, filepath.Join(root, "proc"), 2, "0::/minimega-fc-ffffff/vm1", "/firecracker", "--id", "vm1")

		nukeWrite(t, filepath.Join(root, "sys", "block", "loop0", "loop", "backing_file"), filepath.Join(dir, "vm", "1", "disk-0.cow")+"\n")
		nukeWrite(t, filepath.Join(root, "sys", "block", "loop1", "loop", "backing_file"), "/srv/other.img\n")
		nukeWrite(t, filepath.Join(root, "netns", "mm-fc-a1b2c3-1"), "")
		nukeWrite(t, filepath.Join(root, "netns", "mm-fc-ffffff-1"), "")
		nukeMkdir(t, filepath.Join(root, "cgroup", "minimega-fc-a1b2c3", "vm1"))
		nukeMkdir(t, filepath.Join(root, "cgroup", "minimega-fc-ffffff", "vm1"))

		fcStartupClean()

		want := []string{
			"kill " + fakePID(1),
			"umount " + filepath.Join(vmRoot, "disk0"),
			"run dmsetup ls",
			"run dmsetup remove mm-fc-a1b2c3-1-0",
			"run losetup -d /dev/loop0",
			"netns mm-fc-a1b2c3-1",
		}
		if got := ops.get(); !reflect.DeepEqual(got, want) {
			t.Errorf("operations =\n%q\nwant\n%q", got, want)
		}

		for _, p := range []string{
			filepath.Join(base, "0"),
			filepath.Join(base, "1"),
			filepath.Join(base, "namespaces", "live", "fc0"),
			filepath.Join(base, "namespaces", "old"),
			filepath.Join(dir, "vm", "0"),
			filepath.Join(dir, "jail", "firecracker"),
			filepath.Join(root, "cgroup", "minimega-fc-a1b2c3"),
		} {
			if _, err := os.Lstat(p); !os.IsNotExist(err) {
				t.Errorf("%v exists after the startup cleanup: %v", p, err)
			}
		}

		for _, p := range []string{
			filepath.Join(base, "2", "name"),
			filepath.Join(base, "3"),
			filepath.Join(base, "alias"),
			filepath.Join(base, "minimega.pid"),
			filepath.Join(base, "files"),
			filepath.Join(base, "namespaces", "minimega"),
			filepath.Join(base, "namespaces", "live", "kvm2"),
			filepath.Join(base, "namespaces", "live", "fc3"),
			filepath.Join(dir, "cache", "abc"),
			filepath.Join(root, "cgroup", "minimega-fc-ffffff", "vm1"),
			filepath.Join(root, "proc", fakePID(2)),
		} {
			if _, err := os.Lstat(p); err != nil {
				t.Errorf("%v is missing after the startup cleanup: %v", p, err)
			}
		}
	})

	t.Run("no firecracker directory", func(t *testing.T) {
		_, _, base, ops := setup(t)

		fcStartupClean()

		if got := ops.get(); len(got) != 0 {
			t.Errorf("operations = %q, want none", got)
		}
		for _, name := range []string{"0", "1", "2", "3", "alias", "minimega.pid", "files", "namespaces/live/fc0", "namespaces/old/fc1"} {
			if _, err := os.Lstat(filepath.Join(base, name)); err != nil {
				t.Errorf("%v is missing after the startup cleanup: %v", name, err)
			}
		}
	})
}

func TestFCNukeBaseLinks(t *testing.T) {
	base := t.TempDir()
	vmDir := "/fc/vm"

	for name, target := range map[string]string{
		"4":  "/fc/vm/4",
		"5":  "/fc/vm/../vm/5",
		"6":  "/fc/vmx/6",
		"7":  "/fc/vm",
		"x8": "/fc/vm/8",
	} {
		if err := os.Symlink(target, filepath.Join(base, name)); err != nil {
			t.Fatal(err)
		}
	}
	nukeMkdir(t, filepath.Join(base, "9"))

	if n := fcNukeBaseLinks(base, vmDir); n != 2 {
		t.Errorf("fcNukeBaseLinks() = %v, want 2", n)
	}

	for name, keep := range map[string]bool{"4": false, "5": false, "6": true, "7": true, "x8": true, "9": true} {
		_, err := os.Lstat(filepath.Join(base, name))
		if keep && err != nil {
			t.Errorf("%v is missing: %v", name, err)
		} else if !keep && !os.IsNotExist(err) {
			t.Errorf("%v exists: %v", name, err)
		}
	}

	if n := fcNukeBaseLinks(filepath.Join(base, "missing"), vmDir); n != 0 {
		t.Errorf("fcNukeBaseLinks(missing) = %v, want 0", n)
	}
}

func TestFCNukeCgroupsV1(t *testing.T) {
	root := t.TempDir()
	parent := "minimega-fc-a1b2c3"

	for _, d := range []string{"cpu,cpuacct", "memory"} {
		nukeMkdir(t, filepath.Join(root, d, parent, "vm3"))
	}
	nukeMkdir(t, filepath.Join(root, "memory", "other"))
	if err := os.Symlink("cpu,cpuacct", filepath.Join(root, "cpu")); err != nil {
		t.Fatal(err)
	}

	if n := fcNukeCgroups(root, parent, 1); n != 4 {
		t.Errorf("fcNukeCgroups() = %v, want 4", n)
	}

	for _, d := range []string{"cpu,cpuacct", "memory"} {
		if _, err := os.Lstat(filepath.Join(root, d, parent)); !os.IsNotExist(err) {
			t.Errorf("%v/%v exists after nuke: %v", d, parent, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "memory", "other")); err != nil {
		t.Errorf("memory/other is missing after nuke: %v", err)
	}
}

func TestFCNukeDMNames(t *testing.T) {
	if got := fcNukeDMNames("No devices found\n", "mm-fc-a1b2c3-"); got != nil {
		t.Errorf("fcNukeDMNames(no devices) = %q, want none", got)
	}

	out := "mm-fc-a1b2c3-1-0 (253:0)\nmm-fc-a1b2c30-1-0 (253:1)\n"
	if got, want := fcNukeDMNames(out, "mm-fc-a1b2c3-"), []string{"mm-fc-a1b2c3-1-0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("fcNukeDMNames() = %q, want %q", got, want)
	}
}
