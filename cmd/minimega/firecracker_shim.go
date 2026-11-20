// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// fcJailerShimMagic is the first argument that makes minimega run
// fcJailerShim instead of the normal startup.
const fcJailerShimMagic = "FIRECRACKER-JAILER"

// Tests replace these.
var (
	fcChdir  = unix.Chdir
	fcChroot = unix.Chroot
)

// fcJailerShimCommand returns the command that runs the jailer bin with args
// through fcJailerShim, in a new mount namespace.
func fcJailerShimCommand(bin string, args []string) *exec.Cmd {
	return &exec.Cmd{
		Path: "/proc/self/exe",
		Args: append([]string{os.Args[0], fcJailerShimMagic, fcJailBase(), bin}, args...),
		SysProcAttr: &syscall.SysProcAttr{
			// Go also makes all the mounts in the new namespace private.
			Unshareflags: syscall.CLONE_NEWNS,
		},
	}
}

// fcJailerShimRequested returns true when args, the arguments after the
// flags, are those that fcJailerShimCommand gives.
func fcJailerShimRequested(args []string) bool {
	return len(args) > 2 && args[0] == fcJailerShimMagic
}

// fcJailerShim replaces this process with the jailer bin, with a root whose
// parent mount is private. It does not return.
//
// The jailer calls pivot_root, and the kernel refuses pivot_root with EINVAL
// when the parent mount of the current root is shared. The jailer cannot
// change that mount, because the mount is outside its root. For example,
// Docker with DOCKER_RAMDISK=true puts a container root directly on the host
// root, and a shared volume makes that host root shared in the container.
func fcJailerShim(dir, bin string, args []string) {
	err := fcEnterJailerRoot(dir)
	if err == nil {
		err = unix.Exec(bin, append([]string{bin}, args...), os.Environ())
		err = fmt.Errorf("exec %v: %w", bin, err)
	}

	fmt.Fprintf(os.Stderr, "minimega jailer shim: %v\n", err)
	os.Exit(1)
}

// fcEnterJailerRoot binds / recursively on dir and makes the bind the root of
// this process. The parent of the new root is then the mount that holds dir.
// Inside the new root, dir shows its usual contents, because the kernel copies
// the mounts under / before it mounts the copy on dir.
func fcEnterJailerRoot(dir string) error {
	if err := fcMount("/", dir, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("bind / on %v: %w", dir, err)
	}
	if err := fcChdir(dir); err != nil {
		return fmt.Errorf("enter %v: %w", dir, err)
	}
	if err := fcChroot("."); err != nil {
		return fmt.Errorf("chroot %v: %w", dir, err)
	}

	return fcChdir("/")
}
