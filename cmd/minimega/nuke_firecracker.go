// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/sys/unix"
)

// fcNukeWaitTime is how long nuke waits for killed VMMs to exit.
const fcNukeWaitTime = 5 * time.Second

// Tests replace these.
var (
	nukeProcRoot = "/proc"
	nukeSysBlock = "/sys/block"
	nukeNetnsDir = fcNetnsDir
	nukeRunCmd   = processWrapper
	nukeKill     = func(pid int) error {
		return unix.Kill(pid, unix.SIGKILL)
	}
	nukeUnmount = func(path string) error {
		return unix.Unmount(path, unix.MNT_DETACH)
	}
)

// fcNuke removes what the Firecracker VMs of this minimega instance left on
// the host: the VMM and jailer processes, the mounts in the jails, the
// device-mapper targets, the loop devices, the netns entries, the cgroups,
// the jails, and the instance directories. It keeps the image cache. It
// returns the number of objects that it removed.
func fcNuke() int {
	dir := fcResolve(*f_firecrackerDir)
	jailDir := filepath.Join(dir, "jail")
	vmDir := filepath.Join(dir, "vm")
	prefix := "mm-fc-" + fcInstance + "-"
	parent := fcParentCgroup()

	n := 0

	pids := fcNukeProcs(nukeProcRoot, jailDir, parent)
	for _, pid := range pids {
		log.Info("killing firecracker process: %v", pid)
		if err := nukeKill(pid); err == nil {
			n++
		} else if err != unix.ESRCH {
			log.Error("kill %v: %v", pid, err)
		}
	}
	fcNukeWait(nukeProcRoot, pids, fcNukeWaitTime)

	n += fcNukeUnmount(jailDir, vmDir)
	n += fcNukeDM(prefix)
	n += fcNukeLoops(nukeSysBlock, dir)
	n += fcNukeNetns(nukeNetnsDir, prefix)
	n += fcNukeCgroups(fcCgroupRoot, parent, hostCgroupVersion)

	for _, d := range []string{jailDir, vmDir} {
		n += fcNukeDir(d)
	}

	return n
}

// fcStartupClean removes the Firecracker state that an earlier minimega
// process with the same -base and -firecracker-dir left: the objects that
// fcNuke removes, the <base>/<ID> links to the instance directories, and the
// namespaces/<namespace>/<UUID> links to those <ID> links. It keeps the image
// cache and the other entries of the base directory. It does nothing when the
// Firecracker directory does not exist.
func fcStartupClean() {
	dir := fcResolve(*f_firecrackerDir)
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			log.Warn("unable to remove leftover firecracker state: %v", err)
		}
		return
	}

	vmDir := filepath.Join(dir, "vm")

	n := fcNuke()
	n += fcNukeAliases(*f_base, vmDir)
	n += fcNukeBaseLinks(*f_base, vmDir)

	if n > 0 {
		log.Info("removed %v firecracker objects that an earlier minimega process left in %v and %v", n, *f_base, dir)
	} else {
		log.Debug("no leftover firecracker state in %v and %v", *f_base, dir)
	}
}

// fcNukeAliases removes the namespaces/<namespace>/<UUID> links of base to the
// <ID> links of base that link into vmDir. A later VM can get the same ID, so
// recovery must not find these links. Run it before fcNukeBaseLinks. It
// returns the number of links that it removed.
func fcNukeAliases(base, vmDir string) int {
	nsDir := filepath.Join(base, "namespaces")
	dirs, err := os.ReadDir(nsDir)
	if err != nil {
		log.Debug("unable to list %v: %v", nsDir, err)
		return 0
	}

	n := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}

		dir := filepath.Join(nsDir, d.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Debug("unable to list %v: %v", dir, err)
			continue
		}

		removed := false
		for _, e := range entries {
			if e.Type()&os.ModeSymlink == 0 {
				continue
			}

			p := filepath.Join(dir, e.Name())
			target, err := os.Readlink(p)
			if err != nil || filepath.Dir(filepath.Clean(target)) != filepath.Clean(base) {
				continue
			}
			if _, err := strconv.Atoi(filepath.Base(target)); err != nil {
				continue
			}

			idTarget, err := os.Readlink(target)
			if err != nil || !strings.HasPrefix(filepath.Clean(idTarget), vmDir+"/") {
				continue
			}

			log.Info("removing %v: link to %v", p, target)
			if err := os.Remove(p); err != nil {
				log.Error("remove %v: %v", p, err)
				continue
			}
			removed = true
			n++
		}

		// remove the namespace directory only when it is empty, as Flush does
		if removed {
			os.Remove(dir)
		}
	}

	return n
}

// fcNukeBaseLinks removes the <ID> entries of base that are links into vmDir.
// It returns the number of links that it removed.
func fcNukeBaseLinks(base, vmDir string) int {
	entries, err := os.ReadDir(base)
	if err != nil {
		log.Debug("unable to list %v: %v", base, err)
		return 0
	}

	n := 0
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}

		p := filepath.Join(base, e.Name())
		target, err := os.Readlink(p)
		if err != nil || !strings.HasPrefix(filepath.Clean(target), vmDir+"/") {
			continue
		}

		log.Info("removing %v: link to %v", p, target)
		if err := os.Remove(p); err != nil {
			log.Error("remove %v: %v", p, err)
			continue
		}
		n++
	}

	return n
}

// fcNukeProcs returns the PIDs of the Firecracker and jailer processes of
// this instance in the proc filesystem at root.
func fcNukeProcs(root, jailDir, parent string) []int {
	entries, err := os.ReadDir(root)
	if err != nil {
		log.Error("unable to find firecracker processes: %v", err)
		return nil
	}

	self := os.Getpid()

	var res []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}

		if fcNukeMatch(filepath.Join(root, e.Name()), jailDir, parent) {
			res = append(res, pid)
		}
	}

	return res
}

// fcNukeMatch is true for a process in the parent cgroup or below it, a
// jailer with the jail directory or the parent cgroup in its arguments, or a
// process whose binary is in the jail directory. p is the proc directory of
// the process.
func fcNukeMatch(p, jailDir, parent string) bool {
	if data, err := os.ReadFile(filepath.Join(p, "cgroup")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(line, ":", 3)
			if len(parts) == 3 && (parts[2] == "/"+parent || strings.HasPrefix(parts[2], "/"+parent+"/")) {
				return true
			}
		}
	}

	if data, err := os.ReadFile(filepath.Join(p, "cmdline")); err == nil {
		args := strings.Split(string(data), "\x00")
		for i := 0; i+1 < len(args); i++ {
			switch {
			case args[i] == "--chroot-base-dir" && filepath.Clean(args[i+1]) == jailDir:
				return true
			case args[i] == "--parent-cgroup" && args[i+1] == parent:
				return true
			}
		}
	}

	if exe, err := os.Readlink(filepath.Join(p, "exe")); err == nil && strings.HasPrefix(exe, jailDir+"/") {
		return true
	}

	return false
}

// fcNukeWait waits up to d for the processes to exit.
func fcNukeWait(root string, pids []int, d time.Duration) {
	deadline := time.Now().Add(d)

	for _, pid := range pids {
		for !fcProcGone(root, pid) {
			if time.Now().After(deadline) {
				log.Warn("firecracker process %v did not exit after %v", pid, d)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// fcProcGone is true when the process does not exist or is a zombie.
func fcProcGone(root string, pid int) bool {
	data, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "stat"))
	if err != nil {
		return true
	}

	s := string(data)
	if i := strings.LastIndexByte(s, ')'); i >= 0 && i+2 < len(s) {
		return s[i+2] == 'Z' || s[i+2] == 'X'
	}

	return false
}

// fcNukeUnmount unmounts the mounts below each directory, deepest first. It
// does not unmount a directory itself. It returns the number of unmounts.
func fcNukeUnmount(dirs ...string) int {
	data, err := os.ReadFile(fcMountinfoPath)
	if err != nil {
		log.Error("read mounts: %v", err)
		return 0
	}

	n := 0
	for _, d := range dirs {
		for _, m := range fcMountsUnder(string(data), d) {
			if m == filepath.Clean(d) {
				continue
			}

			log.Info("unmounting %v", m)
			if err := nukeUnmount(m); err == nil {
				n++
			} else if err != unix.EINVAL && err != unix.ENOENT {
				log.Error("unmount %v: %v", m, err)
			}
		}
	}

	return n
}

// fcNukeDM removes the device-mapper targets with names that start with
// prefix. It returns the number of targets that it removed.
func fcNukeDM(prefix string) int {
	out, err := nukeRunCmd("dmsetup", "ls")
	if err != nil {
		log.Debug("dmsetup ls: %v: %v", err, strings.TrimSpace(out))
		return 0
	}

	n := 0
	for _, name := range fcNukeDMNames(out, prefix) {
		log.Info("removing device-mapper target: %v", name)
		if out, err := nukeRunCmd("dmsetup", "remove", name); err != nil {
			log.Error("dmsetup remove %v: %v: %v", name, err, strings.TrimSpace(out))
			continue
		}
		n++
	}

	return n
}

// fcNukeDMNames returns the target names in the output of dmsetup ls that start
// with prefix.
func fcNukeDMNames(out, prefix string) []string {
	var res []string
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) > 0 && strings.HasPrefix(f[0], prefix) {
			res = append(res, f[0])
		}
	}

	return res
}

// fcNukeLoops detaches the loop devices with a backing file in dir.
// sysBlock is the /sys/block directory. It returns the number of loop devices
// that it detached.
func fcNukeLoops(sysBlock, dir string) int {
	entries, err := os.ReadDir(sysBlock)
	if err != nil {
		log.Debug("unable to list loop devices: %v", err)
		return 0
	}

	n := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "loop") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sysBlock, e.Name(), "loop", "backing_file"))
		if err != nil {
			continue
		}

		f := strings.TrimSuffix(strings.TrimSpace(string(data)), " (deleted)")
		if !strings.HasPrefix(f, dir+"/") {
			continue
		}

		dev := "/dev/" + e.Name()
		log.Info("detaching loop device %v: %v", dev, f)
		if out, err := nukeRunCmd("losetup", "-d", dev); err != nil {
			log.Error("losetup -d %v: %v: %v", dev, err, strings.TrimSpace(out))
			continue
		}
		n++
	}

	return n
}

// nukeRemoveNetns detaches the nsfs bind mount of a named network namespace
// and removes its file.
var nukeRemoveNetns = func(dir, name string) error {
	p := filepath.Join(dir, name)
	if err := unix.Unmount(p, unix.MNT_DETACH); err != nil && err != unix.EINVAL && err != unix.ENOENT {
		return fmt.Errorf("unmount netns %v: %v", p, err)
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove netns %v: %v", p, err)
	}
	return nil
}

// fcNukeNetns removes the network namespaces in dir with names that start
// with prefix. It returns the number of namespaces that it removed.
func fcNukeNetns(dir, prefix string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	n := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}

		log.Info("removing netns: %v", e.Name())
		if err := nukeRemoveNetns(dir, e.Name()); err != nil {
			log.Error("%v", err)
			continue
		}
		n++
	}

	return n
}

// fcNukeCgroups removes the VM cgroups below the parent cgroup and the parent
// itself. On cgroup v1, each controller hierarchy has a parent. It returns the
// number of cgroups that it removed.
func fcNukeCgroups(root, parent string, version int) int {
	var dirs []string
	if version == 2 {
		dirs = []string{filepath.Join(root, parent)}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return 0
		}
		for _, e := range entries {
			dirs = append(dirs, filepath.Join(root, e.Name(), parent))
		}
	}

	n := 0
	rmdir := func(d string) {
		log.Info("removing cgroup %v", d)
		if err := fcRmdirRetry(d); err != nil {
			log.Error("remove cgroup %v: %v", d, err)
			return
		}
		n++
	}

	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() {
				rmdir(filepath.Join(d, e.Name()))
			}
		}
		rmdir(d)
	}

	return n
}

// fcNukeDir removes the entries of dir. It keeps an entry that has a mount
// below it. It returns the number of entries that it removed.
func fcNukeDir(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	data, err := os.ReadFile(fcMountinfoPath)
	if err != nil {
		log.Error("not removing the entries of %v: read mounts: %v", dir, err)
		return 0
	}

	n := 0
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if m := fcMountsUnder(string(data), p); len(m) > 0 {
			log.Error("not removing %v: %v is still mounted", p, m[0])
			continue
		}

		log.Info("removing %v", p)
		if err := os.RemoveAll(p); err != nil {
			log.Error("remove %v: %v", p, err)
			continue
		}
		n++
	}

	return n
}
