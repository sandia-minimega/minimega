// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestFCInjectRel(t *testing.T) {
	for in, want := range map[string]string{
		"/etc/hosts":   "etc/hosts",
		"etc/hosts":    "etc/hosts",
		"/a/../b":      "b",
		"//usr//bin/x": "usr/bin/x",
		"/a/./b/":      "a/b",
	} {
		got, err := fcInjectRel(in)
		if err != nil || got != want {
			t.Errorf("fcInjectRel(%q) = %q, %v, want %q", in, got, err, want)
		}
	}

	for _, in := range []string{"", "/", ".", "..", "../x", "/..", "/../x", "/etc/../../x", "a/../../x"} {
		if got, err := fcInjectRel(in); err == nil {
			t.Errorf("fcInjectRel(%q) = %q, want error", in, got)
		}
	}
}

func TestFCParseInjects(t *testing.T) {
	got, err := fcParseInjects([]string{"1", "a:/b", "c d:/e f"})
	if err != nil {
		t.Fatal(err)
	}
	want := FCInjects{
		{Partition: 1, Src: "a", Dst: "/b"},
		{Partition: 1, Src: "c d", Dst: "/e f"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fcParseInjects() = %+v, want %+v", got, want)
	}

	got, err = fcParseInjects([]string{"delete", "0", "/x", "/y"})
	if err != nil {
		t.Fatal(err)
	}
	want = FCInjects{
		{Partition: 0, Dst: "/x", Delete: true},
		{Partition: 0, Dst: "/y", Delete: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fcParseInjects(delete) = %+v, want %+v", got, want)
	}

	for _, args := range [][]string{
		nil,
		{"1"},
		{"delete"},
		{"delete", "1"},
		{"-1", "a:/b"},
		{"x", "a:/b"},
		{"1", "a"},
		{"1", "a:b:c"},
		{"1", ":/b"},
		{"1", "a:"},
		{"1", "a:../b"},
		{"delete", "1", "/etc/../../x"},
	} {
		if got, err := fcParseInjects(args); err == nil {
			t.Errorf("fcParseInjects(%q) = %+v, want error", args, got)
		}
	}
}

func TestFCCheckInjects(t *testing.T) {
	injects := FCInjects{{Partition: 1, Src: "a", Dst: "/b"}}

	if err := fcCheckInjects(true, injects); err != nil {
		t.Errorf("snapshot true: %v", err)
	}
	if err := fcCheckInjects(false, nil); err != nil {
		t.Errorf("no injects: %v", err)
	}
	if err := fcCheckInjects(false, injects); err == nil || err.Error() != "vm config inject needs snapshot true" {
		t.Errorf("snapshot false: error = %v", err)
	}
}

func TestFCInjectInto(t *testing.T) {
	oldIOM := *f_iomBase
	t.Cleanup(func() { *f_iomBase = oldIOM })

	files := t.TempDir()
	*f_iomBase = files

	fcWriteFile(t, filepath.Join(files, "f"), []byte("file"))
	if err := os.Chmod(filepath.Join(files, "f"), 0751); err != nil {
		t.Fatal(err)
	}
	fcWriteFile(t, filepath.Join(files, "d", "g"), []byte("nested"))
	if err := os.Chmod(filepath.Join(files, "d", "g"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("g", filepath.Join(files, "d", "l")); err != nil {
		t.Fatal(err)
	}

	top := t.TempDir()
	mnt := filepath.Join(top, "mnt")
	outside := filepath.Join(top, "outside")
	for _, d := range []string{mnt, outside} {
		if err := os.Mkdir(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	err := fcInjectInto(mnt, []FCInject{
		{Partition: 1, Src: "f", Dst: "/usr/bin/f"},
		{Partition: 1, Src: filepath.Join(files, "d"), Dst: "/opt/d"},
		{Partition: 1, Src: "f", Dst: "/opt/d"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		path string
		data string
		mode os.FileMode
	}{
		{"usr/bin/f", "file", 0751},
		{"opt/d/g", "nested", 0640},
		{"opt/d/f", "file", 0751},
	} {
		p := filepath.Join(mnt, c.path)
		data, err := os.ReadFile(p)
		if err != nil || string(data) != c.data {
			t.Errorf("%v = %q, %v, want %q", c.path, data, err, c.data)
			continue
		}
		if fi, _ := os.Stat(p); fi.Mode().Perm() != c.mode {
			t.Errorf("%v mode = %v, want %v", c.path, fi.Mode().Perm(), c.mode)
		}
	}
	if target, err := os.Readlink(filepath.Join(mnt, "opt/d/l")); err != nil || target != "g" {
		t.Errorf("opt/d/l = %q, %v, want symlink to g", target, err)
	}

	if err := fcInjectInto(mnt, []FCInject{
		{Delete: true, Dst: "/opt/d"},
		{Delete: true, Dst: "/missing"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(mnt, "opt/d")); !os.IsNotExist(err) {
		t.Errorf("opt/d was not deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mnt, "opt")); err != nil {
		t.Errorf("opt was deleted: %v", err)
	}

	// Symlinks in the filesystem must not lead outside it.
	if err := os.Symlink(outside, filepath.Join(mnt, "abs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside", filepath.Join(mnt, "rel")); err != nil {
		t.Fatal(err)
	}
	fcWriteFile(t, filepath.Join(outside, "keep"), []byte("keep"))

	for _, i := range []FCInject{
		{Src: "f", Dst: "/abs/x"},
		{Src: "f", Dst: "/rel/x"},
		{Src: "f", Dst: "/rel/sub/x"},
		{Src: "d", Dst: "/abs/d"},
		{Delete: true, Dst: "/abs/keep"},
		{Delete: true, Dst: "/rel/keep"},
		{Src: "f", Dst: "../x"},
		{Src: "f", Dst: "/etc/../../x"},
		{Delete: true, Dst: "/etc/../../outside"},
	} {
		if err := fcInjectInto(mnt, []FCInject{i}); err == nil {
			t.Errorf("inject %+v: want error", i)
		}
	}

	entries, _ := os.ReadDir(outside)
	if len(entries) != 1 || entries[0].Name() != "keep" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("outside has %q, want only keep", names)
	}
	if _, err := os.Stat(filepath.Join(top, "x")); !os.IsNotExist(err) {
		t.Errorf("inject wrote above the mount point: %v", err)
	}

	// A symlink at the destination is replaced, not followed.
	if err := os.Symlink(filepath.Join(outside, "z"), filepath.Join(mnt, "z")); err != nil {
		t.Fatal(err)
	}
	if err := fcInjectInto(mnt, []FCInject{{Src: "f", Dst: "/z"}}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(mnt, "z")); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("z = %v, %v, want a regular file", fi, err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "z")); !os.IsNotExist(err) {
		t.Errorf("inject followed the symlink at the destination: %v", err)
	}
}

func TestFCApplyInjects(t *testing.T) {
	cmds := setFCDiskSeams(t, func(args []string) (string, error) {
		switch args[0] {
		case "losetup":
			if args[1] == "--find" {
				return "/dev/loop7\n", nil
			}
		case "umount":
			// The mount is not real, so empty the mount point.
			entries, _ := os.ReadDir(args[1])
			for _, e := range entries {
				os.RemoveAll(filepath.Join(args[1], e.Name()))
			}
		}
		return "", nil
	})

	*f_base = t.TempDir()
	*f_iomBase = t.TempDir()
	fcWriteFile(t, filepath.Join(*f_iomBase, "f"), []byte("file"))

	var waited []string
	fcWaitDev = func(dev string, timeout time.Duration) error {
		if timeout != 5*time.Second {
			t.Errorf("timeout = %v, want 5s", timeout)
		}
		waited = append(waited, dev)
		return nil
	}

	err := fcApplyInjects("/fc/vm7/disk-0.raw", FCInjects{
		{Partition: 2, Src: "f", Dst: "/a"},
		{Partition: 0, Src: "f", Dst: "/b"},
		{Partition: 2, Delete: true, Dst: "/a"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"/dev/loop7p2", "/dev/loop7"}; !reflect.DeepEqual(waited, want) {
		t.Errorf("waited for %q, want %q", waited, want)
	}

	calls := cmds.get()
	if len(calls) != 6 {
		t.Fatalf("commands = %q, want 6", calls)
	}

	if want := fcCmd("losetup", "--find", "--show", "--partscan", "/fc/vm7/disk-0.raw"); !reflect.DeepEqual(calls[0], want) {
		t.Errorf("command 0 = %q, want %q", calls[0], want)
	}
	if want := fcCmd("losetup", "-d", "/dev/loop7"); !reflect.DeepEqual(calls[5], want) {
		t.Errorf("command 5 = %q, want %q", calls[5], want)
	}

	for i, dev := range []string{"/dev/loop7p2", "/dev/loop7"} {
		mount, umount := calls[1+2*i], calls[2+2*i]
		if len(mount) != 4 || mount[0] != "mount" || mount[1] != "-w" || mount[2] != dev {
			t.Errorf("mount = %q, want mount -w %v <dir>", mount, dev)
			continue
		}
		if !strings.HasPrefix(mount[3], filepath.Join(*f_base, "fc-inject-")) {
			t.Errorf("mount point %v is not in %v", mount[3], *f_base)
		}
		if want := fcCmd("umount", mount[3]); !reflect.DeepEqual(umount, want) {
			t.Errorf("umount = %q, want %q", umount, want)
		}
		if _, err := os.Stat(mount[3]); !os.IsNotExist(err) {
			t.Errorf("mount point %v was not removed: %v", mount[3], err)
		}
	}

	// The loop device is detached when an inject fails.
	cmds.reset()
	err = fcApplyInjects("/fc/vm7/disk-0.raw", FCInjects{{Partition: 1, Src: "missing", Dst: "/a"}})
	if err == nil {
		t.Fatal("want error")
	}
	calls = cmds.get()
	if want := fcCmd("losetup", "-d", "/dev/loop7"); len(calls) == 0 || !reflect.DeepEqual(calls[len(calls)-1], want) {
		t.Errorf("commands = %q, want detach last", calls)
	}
}

func TestCLIVMConfigInject(t *testing.T) {
	ns := &Namespace{vmConfig: NewVMConfig()}

	run := func(args ...string) (*minicli.Response, error) {
		c := &minicli.Command{ListArgs: map[string][]string{}}
		if len(args) > 0 {
			c.ListArgs["injectspec"] = args
		}
		resp := &minicli.Response{}
		return resp, cliVMConfigInject(ns, c, resp)
	}

	if resp, err := run(); err != nil || len(resp.Tabular) != 0 || resp.Response != "" {
		t.Errorf("empty config: %+v, %v", resp, err)
	}

	if _, err := run("1", "a:/b", "c d:/e f"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("delete", "2", "/x"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("x", "a:/b"); err == nil {
		t.Error("invalid partition: want error")
	}

	resp, err := run()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"partition", "action", "src", "dst"}; !reflect.DeepEqual(resp.Header, want) {
		t.Errorf("header = %q, want %q", resp.Header, want)
	}
	if want := [][]string{
		{"1", "copy", "a", "/b"},
		{"1", "copy", "c d", "/e f"},
		{"2", "delete", "", "/x"},
	}; !reflect.DeepEqual(resp.Tabular, want) {
		t.Errorf("rows = %q, want %q", resp.Tabular, want)
	}

	if got, err := ns.vmConfig.FirecrackerConfig.Info("inject"); err != nil || got != "[1 a:/b, 1 c d:/e f, delete 2 /x]" {
		t.Errorf("Info(inject) = %q, %v", got, err)
	}

	// The copy does not share entries.
	cp := ns.vmConfig.Copy()
	cp.Injects[0].Dst = "/changed"
	if ns.vmConfig.Injects[0].Dst != "/b" {
		t.Error("Copy() shares the inject entries")
	}

	var buf bytes.Buffer
	if err := ns.vmConfig.FirecrackerConfig.WriteConfig(&buf); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"vm config inject 1 a:/b\n",
		"vm config inject 1 \"c d:/e f\"\n",
		"vm config inject delete 2 /x\n",
	} {
		if !strings.Contains(buf.String(), line) {
			t.Errorf("WriteConfig() = %q, want line %q", buf.String(), line)
		}
	}

	cfg := NewVMConfig()
	if err := cfg.FirecrackerConfig.ReadConfig(strings.NewReader(buf.String()), DefaultNamespace, &cfg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Injects, ns.vmConfig.Injects) {
		t.Errorf("round trip = %+v, want %+v", cfg.Injects, ns.vmConfig.Injects)
	}

	ns.vmConfig.Clear("inject")
	if len(ns.vmConfig.Injects) != 0 {
		t.Errorf("clear vm config inject left %+v", ns.vmConfig.Injects)
	}
	if resp, err := run(); err != nil || len(resp.Tabular) != 0 {
		t.Errorf("after clear: %+v, %v", resp, err)
	}
}
