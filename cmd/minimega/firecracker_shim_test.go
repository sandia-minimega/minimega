// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestFCJailerShimCommand(t *testing.T) {
	dir := t.TempDir()
	setFCJailGlobals(t, dir)

	cmd := fcJailerShimCommand("/usr/bin/jailer", []string{"--id", "vm3", "--", "--api-sock", "/api.sock"})

	if cmd.Path != "/proc/self/exe" {
		t.Errorf("Path = %q, want /proc/self/exe", cmd.Path)
	}
	want := []string{os.Args[0], fcJailerShimMagic, filepath.Join(fcResolve(dir), "jail"), "/usr/bin/jailer", "--id", "vm3", "--", "--api-sock", "/api.sock"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.Unshareflags != syscall.CLONE_NEWNS {
		t.Errorf("SysProcAttr = %+v, want Unshareflags CLONE_NEWNS", cmd.SysProcAttr)
	}
	if !fcJailerShimRequested(cmd.Args[1:]) {
		t.Errorf("fcJailerShimRequested(%q) = false, want true", cmd.Args[1:])
	}
}

func TestFCJailerShimRequested(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{fcJailerShimMagic},
		{fcJailerShimMagic, "/fc/jail"},
		{"CONTAINER", "/fc/jail", "/usr/bin/jailer"},
		{"vm", "info", "x"},
	} {
		if fcJailerShimRequested(args) {
			t.Errorf("fcJailerShimRequested(%q) = true, want false", args)
		}
	}
	if !fcJailerShimRequested([]string{fcJailerShimMagic, "/fc/jail", "/usr/bin/jailer"}) {
		t.Error("fcJailerShimRequested with a jailer and no jailer args = false, want true")
	}
}

func TestFCEnterJailerRoot(t *testing.T) {
	setFCJailGlobals(t, t.TempDir())
	oldChdir, oldChroot := fcChdir, fcChroot
	t.Cleanup(func() { fcChdir, fcChroot = oldChdir, oldChroot })

	var calls []string
	fcMount = func(src, dst, fstype string, flags uintptr, data string) error {
		if flags != unix.MS_BIND|unix.MS_REC {
			t.Errorf("mount flags = %#x, want MS_BIND|MS_REC", flags)
		}
		calls = append(calls, "mount "+src+" "+dst)
		return nil
	}
	fcChdir = func(dir string) error {
		calls = append(calls, "chdir "+dir)
		return nil
	}
	fcChroot = func(dir string) error {
		calls = append(calls, "chroot "+dir)
		return nil
	}

	if err := fcEnterJailerRoot("/fc/jail"); err != nil {
		t.Fatal(err)
	}
	want := []string{"mount / /fc/jail", "chdir /fc/jail", "chroot .", "chdir /"}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %q, want %q", calls, want)
	}

	calls = nil
	fcMount = func(string, string, string, uintptr, string) error { return unix.EPERM }
	err := fcEnterJailerRoot("/fc/jail")
	if !errors.Is(err, unix.EPERM) || !strings.Contains(err.Error(), "bind / on /fc/jail") {
		t.Errorf("err = %v, want a bind error that wraps EPERM", err)
	}
	if len(calls) != 0 {
		t.Errorf("calls after a failed bind = %q, want none", calls)
	}
}
