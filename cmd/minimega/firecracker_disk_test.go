// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

type fcCmdLog struct {
	mu    sync.Mutex
	calls [][]string
}

func (l *fcCmdLog) add(args []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, append([]string(nil), args...))
}

func (l *fcCmdLog) get() [][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([][]string(nil), l.calls...)
}

func (l *fcCmdLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = nil
}

// setFCDiskSeams replaces the external commands and host checks of the disk
// code. reply answers each command.
func setFCDiskSeams(t *testing.T, reply func(args []string) (string, error)) *fcCmdLog {
	t.Helper()

	setFCJailGlobals(t, t.TempDir())

	oldRun, oldReflink, oldDM := fcRunCmd, fcReflinkOK, fcDMUsable
	oldClone, oldFill, oldCache := fcCloneFile, fcCacheFill, fcCache
	oldWait, oldBase, oldIOM := fcWaitDev, *f_base, *f_iomBase
	t.Cleanup(func() {
		fcRunCmd, fcReflinkOK, fcDMUsable = oldRun, oldReflink, oldDM
		fcCloneFile, fcCacheFill, fcCache = oldClone, oldFill, oldCache
		fcWaitDev, *f_base, *f_iomBase = oldWait, oldBase, oldIOM
	})

	cmds := &fcCmdLog{}
	fcRunCmd = func(args ...string) (string, error) {
		cmds.add(args)
		if reply != nil {
			return reply(args)
		}
		return "", nil
	}
	fcReflinkOK = func() bool { return false }
	fcDMUsable = func() bool { return false }
	fcChown = func(string, int, int) error { return nil }
	fcCache = newFCCacheState()

	return cmds
}

func fcWriteFile(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func fcImageJSON(t *testing.T, images ...fcImage) string {
	t.Helper()
	b, err := json.Marshal(images)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFCCacheKey(t *testing.T) {
	id := fcSourceID{Path: "/img/a.raw", Size: 100, Mtime: 5, Inode: 7}

	k := fcCacheKey(id)
	if len(k) != 16 {
		t.Fatalf("fcCacheKey() = %q, want 16 characters", k)
	}
	if _, err := hex.DecodeString(k); err != nil {
		t.Errorf("fcCacheKey() = %q, want hex: %v", k, err)
	}
	if fcCacheKey(id) != k {
		t.Error("fcCacheKey() is not stable")
	}

	for name, change := range map[string]func(*fcSourceID){
		"path":  func(i *fcSourceID) { i.Path = "/img/b.raw" },
		"size":  func(i *fcSourceID) { i.Size++ },
		"mtime": func(i *fcSourceID) { i.Mtime++ },
		"inode": func(i *fcSourceID) { i.Inode++ },
	} {
		id2 := id
		change(&id2)
		if fcCacheKey(id2) == k {
			t.Errorf("fcCacheKey() did not change with %v", name)
		}
	}

	src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.raw"), []byte("data"))

	a, err := fcStatSource(src)
	if err != nil {
		t.Fatal(err)
	}
	b, err := fcStatSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if fcCacheKey(a) != fcCacheKey(b) {
		t.Error("key of an unchanged file changed")
	}
	if a.Inode == 0 {
		t.Error("fcStatSource() did not read the inode")
	}

	mtime := time.Unix(0, a.Mtime).Add(time.Second)
	if err := os.Chtimes(src, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	c, err := fcStatSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if fcCacheKey(c) == fcCacheKey(a) {
		t.Error("key did not change with the mtime of the file")
	}

	// A new file at the same path has a new inode.
	tmp := fcWriteFile(t, src+".new", []byte("data"))
	if err := os.Chtimes(tmp, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, src); err != nil {
		t.Fatal(err)
	}
	d, err := fcStatSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if d.Inode == c.Inode {
		t.Skip("filesystem reused the inode")
	}
	if fcCacheKey(d) == fcCacheKey(c) {
		t.Error("key did not change with the inode of the file")
	}
}

func TestFCCacheInflight(t *testing.T) {
	setFCDiskSeams(t, nil)

	src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.qcow2"), []byte("qcow2"))

	var fills int32
	fcCacheFill = func(src, dst, format string) error {
		atomic.AddInt32(&fills, 1)
		time.Sleep(50 * time.Millisecond)
		return os.WriteFile(dst, []byte("raw"), 0600)
	}

	const n = 8
	keys := make([]string, n)
	paths := make([]string, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			keys[i], paths[i], errs[i] = fcCache.get(src, "qcow2", i)
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&fills); got != 1 {
		t.Errorf("fills = %v, want 1", got)
	}

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %v: %v", i, errs[i])
		}
		if paths[i] != paths[0] || keys[i] != keys[0] {
			t.Errorf("caller %v got %v, want %v", i, paths[i], paths[0])
		}
	}

	if want := filepath.Join(fcCacheDir(), keys[0]+".raw"); paths[0] != want {
		t.Errorf("path = %v, want %v", paths[0], want)
	}
	if got := fcCache.inUse(keys[0]); got != n {
		t.Errorf("inUse() = %v, want %v", got, n)
	}

	data, err := os.ReadFile(filepath.Join(fcCacheDir(), keys[0]+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta fcCacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Source != fcResolve(src) || meta.Format != "qcow2" || meta.Size != 5 || meta.Inode == 0 || meta.Created.IsZero() {
		t.Errorf("metadata = %+v", meta)
	}

	entries, err := os.ReadDir(fcCacheDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temporary file %v was not renamed", e.Name())
		}
	}
}

func TestFCCacheFillError(t *testing.T) {
	setFCDiskSeams(t, nil)

	src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.qcow2"), []byte("qcow2"))

	fcCacheFill = func(src, dst, format string) error {
		os.WriteFile(dst, []byte("partial"), 0600)
		return fmt.Errorf("convert failed")
	}

	if _, _, err := fcCache.get(src, "qcow2", 1); err == nil || !strings.Contains(err.Error(), "convert failed") {
		t.Fatalf("get() error = %v, want convert failed", err)
	}

	entries, _ := os.ReadDir(fcCacheDir())
	if len(entries) != 0 {
		t.Errorf("cache has %v files after a failed fill, want 0", len(entries))
	}
}

func TestFCDiskChainCache(t *testing.T) {
	dir := t.TempDir()
	overlay := fcWriteFile(t, filepath.Join(dir, "ov.qcow2"), []byte("overlay"))
	base := fcWriteFile(t, filepath.Join(dir, "base.raw"), []byte("base"))
	missing := filepath.Join(dir, "missing.qcow2")

	info := fcImageJSON(t,
		fcImage{Filename: overlay, Format: "qcow2", BackingFilename: "base.raw"},
		fcImage{Filename: "base.raw", Format: "raw"},
	)
	cmds := setFCDiskSeams(t, func(args []string) (string, error) {
		return info, nil
	})

	chain := func(src string, wantCmds int) {
		t.Helper()
		c, err := fcDiskChain(src)
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 2 || c[0].Format != "qcow2" || c[1].Format != "raw" {
			t.Errorf("fcDiskChain(%v) = %+v", src, c)
		}
		c[0].Format = "changed"
		if n := len(cmds.get()); n != wantCmds {
			t.Errorf("qemu-img commands = %d, want %d", n, wantCmds)
		}
	}

	chain(overlay, 1)
	chain(overlay, 1)

	// A change to a backing file or to the source runs qemu-img again.
	fcWriteFile(t, base, []byte("new base"))
	chain(overlay, 2)
	chain(overlay, 2)
	fcWriteFile(t, overlay, []byte("new overlay"))
	chain(overlay, 3)

	// A file that cannot be read is not kept.
	chain(missing, 4)
	chain(missing, 5)
}

func TestStatCache(t *testing.T) {
	dir := t.TempDir()
	a := fcWriteFile(t, filepath.Join(dir, "a"), []byte("a"))

	c := newStatCache[int]()
	before, err := fcStatSource(a)
	if err != nil {
		t.Fatal(err)
	}

	fcWriteFile(t, a, []byte("changed"))
	c.put(a, before, 1)
	if _, ok := c.get(a); ok {
		t.Error("get() found a value that put made from an old version of the file")
	}

	before, _ = fcStatSource(a)
	c.put(a, before, 2)
	if v, ok := c.get(a); !ok || v != 2 {
		t.Errorf("get() = %v, %v, want 2, true", v, ok)
	}

	// A path that becomes a link to another file is a new version.
	b := fcWriteFile(t, filepath.Join(dir, "b"), []byte("changed"))
	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(b, a); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.get(a); ok {
		t.Error("get() found a value after the path changed to a link")
	}

	c = newStatCache[int]()
	for i := 0; i < fcStatCacheMax+10; i++ {
		p := fcWriteFile(t, filepath.Join(dir, "many", fmt.Sprint(i)), nil)
		id, err := fcStatSource(p)
		if err != nil {
			t.Fatal(err)
		}
		c.put(p, id, i)
	}
	if n := len(c.m); n != fcStatCacheMax {
		t.Errorf("cache has %d entries, want %d", n, fcStatCacheMax)
	}
}

func TestFCPlanDisk(t *testing.T) {
	raw := fcImage{Filename: "/img/base.raw", Format: "raw", VirtualSize: 1 << 20}
	plain := fcImage{Filename: "/img/base.qcow2", Format: "qcow2", VirtualSize: 1 << 20}
	overlay := fcImage{Filename: "/img/node.qcow2", Format: "qcow2", VirtualSize: 1 << 20, BackingFilename: "base.raw", FullBackingFilename: "/img/base.raw"}
	overlayRel := fcImage{Filename: "/img/node.qcow2", Format: "qcow2", BackingFilename: "base.raw"}
	mid := fcImage{Filename: "/img/mid.qcow2", Format: "qcow2", BackingFilename: "base.raw", FullBackingFilename: "/img/base.raw"}
	top := fcImage{Filename: "/img/top.qcow2", Format: "qcow2", BackingFilename: "mid.qcow2", FullBackingFilename: "/img/mid.qcow2"}
	vmdk := fcImage{Filename: "/img/node.vmdk", Format: "vmdk", BackingFilename: "base.raw"}

	for _, tc := range []struct {
		name  string
		chain []fcImage
		mode  fcDiskMode
		base  string
	}{
		{"raw", []fcImage{raw}, fcDiskCached, "/img/base.raw"},
		{"qcow2", []fcImage{plain}, fcDiskCached, "/img/base.qcow2"},
		{"overlay on raw", []fcImage{overlay, raw}, fcDiskOverlay, "/img/base.raw"},
		{"overlay on qcow2", []fcImage{top, {Filename: "/img/mid.qcow2", Format: "qcow2"}}, fcDiskOverlay, "/img/mid.qcow2"},
		{"relative backing", []fcImage{overlayRel, {Filename: "base.raw", Format: "raw"}}, fcDiskOverlay, "/img/base.raw"},
		{"deep chain", []fcImage{top, mid, raw}, fcDiskConvert, ""},
		{"overlay not qcow2", []fcImage{vmdk, raw}, fcDiskConvert, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain, err := fcParseChain("src", fcImageJSON(t, tc.chain...))
			if err != nil {
				t.Fatal(err)
			}

			mode, base, err := fcPlanDisk(chain)
			if err != nil {
				t.Fatal(err)
			}
			if mode != tc.mode {
				t.Errorf("mode = %v, want %v", mode, tc.mode)
			}
			if mode != fcDiskConvert && base.Filename != tc.base {
				t.Errorf("base = %v, want %v", base.Filename, tc.base)
			}
		})
	}

	if _, err := fcParseChain("src", "[]"); err == nil {
		t.Error("fcParseChain([]): want error")
	}
	if _, err := fcParseChain("src", "not json"); err == nil {
		t.Error("fcParseChain(not json): want error")
	}
}

func TestFCDMSnapshotTable(t *testing.T) {
	if got, want := fcDMSnapshotTable(1<<30, "/dev/loop0", "/dev/loop1"), "0 2097152 snapshot /dev/loop0 /dev/loop1 P 8"; got != want {
		t.Errorf("fcDMSnapshotTable() = %q, want %q", got, want)
	}

	old := fcInstance
	defer func() { fcInstance = old }()
	fcInstance = "a1b2c3"

	if got, want := fcDMName(7, 1), "mm-fc-a1b2c3-7-1"; got != want {
		t.Errorf("fcDMName() = %q, want %q", got, want)
	}
}

func TestFCDMNode(t *testing.T) {
	for _, tc := range []struct {
		name               string
		mknodesErr, waitOK bool
	}{
		{"node exists", false, true},
		{"mknodes fails", true, true},
		{"node missing", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmds := setFCDiskSeams(t, func(args []string) (string, error) {
				if tc.mknodesErr {
					return "no control", fmt.Errorf("exit status 1")
				}
				return "", nil
			})

			var waited []string
			fcWaitDev = func(dev string, timeout time.Duration) error {
				if timeout != fcDMNodeTimeout {
					t.Errorf("timeout = %v, want %v", timeout, fcDMNodeTimeout)
				}
				waited = append(waited, dev)
				if !tc.waitOK {
					return fmt.Errorf("%v did not appear after %v", dev, timeout)
				}
				return nil
			}

			p, err := fcDMNode("mm-fc-a1b2c3-7-0")

			if want := [][]string{fcCmd("dmsetup", "mknodes", "mm-fc-a1b2c3-7-0")}; !reflect.DeepEqual(cmds.get(), want) {
				t.Errorf("commands = %q, want %q", cmds.get(), want)
			}
			if want := []string{"/dev/mapper/mm-fc-a1b2c3-7-0"}; !reflect.DeepEqual(waited, want) {
				t.Errorf("waited for %q, want %q", waited, want)
			}

			if tc.waitOK {
				if err != nil || p != "/dev/mapper/mm-fc-a1b2c3-7-0" {
					t.Errorf("fcDMNode() = %q, %v", p, err)
				}
			} else if err == nil || p != "" || !strings.Contains(err.Error(), "/dev/mapper/mm-fc-a1b2c3-7-0") {
				t.Errorf("fcDMNode() = %q, %v, want an error that names the node", p, err)
			}
		})
	}
}

func TestFCWaitDev(t *testing.T) {
	dev := filepath.Join(t.TempDir(), "mm-fc-a1b2c3-7-0")

	go func() {
		time.Sleep(100 * time.Millisecond)
		os.WriteFile(dev, nil, 0600)
	}()
	if err := fcWaitDev(dev, 5*time.Second); err != nil {
		t.Errorf("fcWaitDev() for a node that appears later = %v", err)
	}

	missing := filepath.Join(t.TempDir(), "missing")
	if err := fcWaitDev(missing, 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), missing) {
		t.Errorf("fcWaitDev() for a missing node = %v, want an error that names it", err)
	}
}

func TestFCCacheType(t *testing.T) {
	for in, want := range map[string]string{
		"":             "Writeback",
		"unsafe":       "Unsafe",
		"writeback":    "Writeback",
		"none":         "Writeback",
		"directsync":   "Writeback",
		"writethrough": "Writeback",
	} {
		if got := fcCacheType(in); got != want {
			t.Errorf("fcCacheType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFCSparseCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")

	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteAt([]byte("start"), 0)
	f.WriteAt([]byte("middle"), 1<<20+5)
	f.Truncate(3 << 20)
	f.Close()

	dst := filepath.Join(dir, "dst")
	if err := fcSparseCopy(src, dst); err != nil {
		t.Fatal(err)
	}

	a, _ := os.ReadFile(src)
	b, _ := os.ReadFile(dst)
	if !bytes.Equal(a, b) {
		t.Error("copy differs from source")
	}
}

func fcCmd(args ...string) []string { return args }

func TestFCPrepareDisk(t *testing.T) {
	fcDiskReply := func(info string) func([]string) (string, error) {
		return func(args []string) (string, error) {
			switch {
			case args[0] == "qemu-img" && args[1] == "info":
				return info, nil
			case args[0] == "losetup" && len(args) == 5 && args[3] == "--read-only":
				return "/dev/loop3\n", nil
			case args[0] == "losetup" && args[1] == "--find":
				return "/dev/loop4\n", nil
			}
			return "", nil
		}
	}

	fakeFill := func(fills *int32) func(src, dst, format string) error {
		return func(src, dst, format string) error {
			atomic.AddInt32(fills, 1)
			f, err := os.Create(dst)
			if err != nil {
				return err
			}
			f.WriteAt([]byte("base"), 0)
			f.Truncate(1 << 20)
			return f.Close()
		}
	}

	info := func(t *testing.T, src, format string) string {
		return fcImageJSON(t, fcImage{Filename: src, Format: format, VirtualSize: 1 << 20})
	}

	t.Run("reflink", func(t *testing.T) {
		src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.qcow2"), []byte("qcow2"))
		cmds := setFCDiskSeams(t, nil)
		fcRunCmd = func(args ...string) (string, error) {
			cmds.add(args)
			return fcDiskReply(info(t, src, "qcow2"))(args)
		}

		var fills int32
		fcCacheFill = fakeFill(&fills)
		fcReflinkOK = func() bool { return true }

		var clones [][2]string
		fcCloneFile = func(src, dst string) error {
			clones = append(clones, [2]string{src, dst})
			return os.WriteFile(dst, nil, 0600)
		}

		var chowns []string
		fcChown = func(p string, uid, gid int) error {
			chowns = append(chowns, fmt.Sprintf("%v:%d:%d", p, uid, gid))
			return nil
		}

		inst := t.TempDir()
		d, err := fcPrepareDisk(inst, 0, DiskConfig{Path: src, Cache: "unsafe", Interface: "ide"}, true, 7, 200007)
		if err != nil {
			t.Fatal(err)
		}

		id, _ := fcStatSource(src)
		key := fcCacheKey(id)
		dst := filepath.Join(inst, "disk-0.raw")

		if want := [][]string{fcCmd("qemu-img", "info", "--output=json", "--backing-chain", fcResolve(src))}; !reflect.DeepEqual(cmds.get(), want) {
			t.Errorf("commands =\n%q\nwant\n%q", cmds.get(), want)
		}
		if want := [][2]string{{filepath.Join(fcCacheDir(), key+".raw"), dst}}; !reflect.DeepEqual(clones, want) {
			t.Errorf("clones = %q, want %q", clones, want)
		}
		if want := []string{dst + ":200007:200007"}; !reflect.DeepEqual(chowns, want) {
			t.Errorf("chowns = %q, want %q", chowns, want)
		}
		if d.HostPath != dst || d.IsBlock || d.ReadOnly || d.CacheType != "Unsafe" {
			t.Errorf("disk = %+v", d)
		}
		if fills != 1 {
			t.Errorf("fills = %v, want 1", fills)
		}
		if got := fcCache.inUse(key); got != 1 {
			t.Errorf("inUse() = %v, want 1", got)
		}

		cmds.reset()
		if err := fcReleaseDisk(d); err != nil {
			t.Fatal(err)
		}
		if len(cmds.get()) != 0 {
			t.Errorf("release commands = %q, want none", cmds.get())
		}
		if got := fcCache.inUse(key); got != 0 {
			t.Errorf("inUse() after release = %v, want 0", got)
		}
	})

	t.Run("device-mapper", func(t *testing.T) {
		src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.raw"), []byte("raw"))
		cmds := setFCDiskSeams(t, nil)
		fcRunCmd = func(args ...string) (string, error) {
			cmds.add(args)
			return fcDiskReply(info(t, src, "raw"))(args)
		}

		var fills int32
		fcCacheFill = fakeFill(&fills)
		fcDMUsable = func() bool { return true }

		var waited []string
		fcWaitDev = func(dev string, timeout time.Duration) error {
			if timeout != fcDMNodeTimeout {
				t.Errorf("timeout = %v, want %v", timeout, fcDMNodeTimeout)
			}
			waited = append(waited, dev)
			return nil
		}

		inst1, inst2 := t.TempDir(), t.TempDir()
		d1, err := fcPrepareDisk(inst1, 0, DiskConfig{Path: src}, true, 7, 200007)
		if err != nil {
			t.Fatal(err)
		}
		d2, err := fcPrepareDisk(inst2, 1, DiskConfig{Path: src}, true, 8, 200008)
		if err != nil {
			t.Fatal(err)
		}

		id, _ := fcStatSource(src)
		key := fcCacheKey(id)
		cached := filepath.Join(fcCacheDir(), key+".raw")
		infoCmd := fcCmd("qemu-img", "info", "--output=json", "--backing-chain", fcResolve(src))
		cow1, cow2 := filepath.Join(inst1, "disk-0.cow"), filepath.Join(inst2, "disk-1.cow")

		want := [][]string{
			infoCmd,
			fcCmd("losetup", "--find", "--show", "--read-only", cached),
			fcCmd("losetup", "--find", "--show", cow1),
			fcCmd("dmsetup", "create", "mm-fc-a1b2c3-7-0", "--table", "0 2048 snapshot /dev/loop3 /dev/loop4 P 8"),
			fcCmd("dmsetup", "mknodes", "mm-fc-a1b2c3-7-0"),
			fcCmd("losetup", "--find", "--show", cow2),
			fcCmd("dmsetup", "create", "mm-fc-a1b2c3-8-1", "--table", "0 2048 snapshot /dev/loop3 /dev/loop4 P 8"),
			fcCmd("dmsetup", "mknodes", "mm-fc-a1b2c3-8-1"),
		}
		if !reflect.DeepEqual(cmds.get(), want) {
			t.Errorf("commands =\n%q\nwant\n%q", cmds.get(), want)
		}
		if want := []string{"/dev/mapper/mm-fc-a1b2c3-7-0", "/dev/mapper/mm-fc-a1b2c3-8-1"}; !reflect.DeepEqual(waited, want) {
			t.Errorf("waited for %q, want %q", waited, want)
		}

		if d1.HostPath != "/dev/mapper/mm-fc-a1b2c3-7-0" || !d1.IsBlock || d1.CacheType != "Writeback" {
			t.Errorf("disk 1 = %+v", d1)
		}
		if fi, err := os.Stat(cow1); err != nil || fi.Size() < 1<<20 {
			t.Errorf("COW file: %v, %v", fi, err)
		}
		if fills != 1 {
			t.Errorf("fills = %v, want 1", fills)
		}
		if got := fcCache.inUse(key); got != 2 {
			t.Errorf("inUse() = %v, want 2", got)
		}

		cmds.reset()
		if err := fcReleaseDisk(d1); err != nil {
			t.Fatal(err)
		}
		if err := fcReleaseDisk(d2); err != nil {
			t.Fatal(err)
		}

		want = [][]string{
			fcCmd("dmsetup", "remove", "mm-fc-a1b2c3-7-0"),
			fcCmd("losetup", "-d", "/dev/loop4"),
			fcCmd("dmsetup", "remove", "mm-fc-a1b2c3-8-1"),
			fcCmd("losetup", "-d", "/dev/loop4"),
			fcCmd("losetup", "-d", "/dev/loop3"),
		}
		if !reflect.DeepEqual(cmds.get(), want) {
			t.Errorf("release commands =\n%q\nwant\n%q", cmds.get(), want)
		}
		if got := fcCache.inUse(key); got != 0 {
			t.Errorf("inUse() after release = %v, want 0", got)
		}
	})

	t.Run("device-mapper node missing", func(t *testing.T) {
		src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.raw"), []byte("raw"))
		cmds := setFCDiskSeams(t, nil)
		fcRunCmd = func(args ...string) (string, error) {
			cmds.add(args)
			return fcDiskReply(info(t, src, "raw"))(args)
		}

		var fills int32
		fcCacheFill = fakeFill(&fills)
		fcDMUsable = func() bool { return true }
		fcWaitDev = func(dev string, timeout time.Duration) error {
			return fmt.Errorf("%v did not appear after %v", dev, timeout)
		}

		_, err := fcPrepareDisk(t.TempDir(), 0, DiskConfig{Path: src}, true, 7, 200007)
		if err == nil || !strings.Contains(err.Error(), "/dev/mapper/mm-fc-a1b2c3-7-0") {
			t.Fatalf("error = %v, want one that names /dev/mapper/mm-fc-a1b2c3-7-0", err)
		}

		calls := cmds.get()
		want := [][]string{
			fcCmd("dmsetup", "mknodes", "mm-fc-a1b2c3-7-0"),
			fcCmd("dmsetup", "remove", "mm-fc-a1b2c3-7-0"),
			fcCmd("losetup", "-d", "/dev/loop4"),
			fcCmd("losetup", "-d", "/dev/loop3"),
		}
		if len(calls) < len(want) || !reflect.DeepEqual(calls[len(calls)-len(want):], want) {
			t.Errorf("commands = %q, want cleanup %q", calls, want)
		}
	})

	t.Run("full copy", func(t *testing.T) {
		src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.raw"), []byte("raw"))
		cmds := setFCDiskSeams(t, nil)
		fcRunCmd = func(args ...string) (string, error) {
			cmds.add(args)
			return fcDiskReply(info(t, src, "raw"))(args)
		}

		var fills int32
		fcCacheFill = fakeFill(&fills)

		inst := t.TempDir()
		d, err := fcPrepareDisk(inst, 2, DiskConfig{Path: src}, true, 7, 200007)
		if err != nil {
			t.Fatal(err)
		}

		id, _ := fcStatSource(src)
		a, _ := os.ReadFile(filepath.Join(fcCacheDir(), fcCacheKey(id)+".raw"))
		b, _ := os.ReadFile(filepath.Join(inst, "disk-2.raw"))
		if len(a) != 1<<20 || !bytes.Equal(a, b) {
			t.Errorf("copy has %v bytes, cache has %v, or they differ", len(b), len(a))
		}
		if d.HostPath != filepath.Join(inst, "disk-2.raw") || d.IsBlock {
			t.Errorf("disk = %+v", d)
		}
		if len(cmds.get()) != 1 {
			t.Errorf("commands = %q, want qemu-img info only", cmds.get())
		}
	})

	t.Run("overlay", func(t *testing.T) {
		imgs := t.TempDir()
		base := fcWriteFile(t, filepath.Join(imgs, "base.raw"), []byte("base"))
		overlay := fcWriteFile(t, filepath.Join(imgs, "node.qcow2"), []byte("overlay"))

		chain := fcImageJSON(t,
			fcImage{Filename: overlay, Format: "qcow2", BackingFilename: "base.raw", FullBackingFilename: base},
			fcImage{Filename: base, Format: "raw"},
		)

		cmds := setFCDiskSeams(t, nil)
		inst := t.TempDir()
		ov := filepath.Join(inst, "disk-0.overlay.qcow2")
		fcRunCmd = func(args ...string) (string, error) {
			cmds.add(args)
			if args[0] == "qemu-img" && args[1] == "commit" {
				if data, err := os.ReadFile(ov); err != nil || string(data) != "overlay" {
					return "", fmt.Errorf("overlay copy: %q, %v", data, err)
				}
			}
			return fcDiskReply(chain)(args)
		}

		var fills int32
		fcCacheFill = fakeFill(&fills)
		fcDMUsable = func() bool { return true }

		d, err := fcPrepareDisk(inst, 0, DiskConfig{Path: overlay}, true, 7, 200007)
		if err != nil {
			t.Fatal(err)
		}

		dst := filepath.Join(inst, "disk-0.raw")
		want := [][]string{
			fcCmd("qemu-img", "info", "--output=json", "--backing-chain", fcResolve(overlay)),
			fcCmd("qemu-img", "rebase", "-u", "-F", "raw", "-b", dst, ov),
			fcCmd("qemu-img", "commit", ov),
		}
		if !reflect.DeepEqual(cmds.get(), want) {
			t.Errorf("commands =\n%q\nwant\n%q", cmds.get(), want)
		}
		if _, err := os.Stat(ov); !os.IsNotExist(err) {
			t.Errorf("overlay copy was not removed: %v", err)
		}
		if fi, err := os.Stat(dst); err != nil || fi.Size() != 1<<20 {
			t.Errorf("disk: %v, %v", fi, err)
		}
		if d.HostPath != dst || d.IsBlock {
			t.Errorf("disk = %+v", d)
		}

		id, _ := fcStatSource(base)
		if got := fcCache.inUse(fcCacheKey(id)); got != 1 {
			t.Errorf("inUse(base) = %v, want 1", got)
		}
	})

	t.Run("deep chain", func(t *testing.T) {
		imgs := t.TempDir()
		top := fcWriteFile(t, filepath.Join(imgs, "top.qcow2"), []byte("top"))

		chain := fcImageJSON(t,
			fcImage{Filename: top, Format: "qcow2", BackingFilename: "mid.qcow2"},
			fcImage{Filename: filepath.Join(imgs, "mid.qcow2"), Format: "qcow2", BackingFilename: "base.raw"},
			fcImage{Filename: filepath.Join(imgs, "base.raw"), Format: "raw"},
		)

		cmds := setFCDiskSeams(t, func(args []string) (string, error) {
			return fcDiskReply(chain)(args)
		})

		var fills int32
		fcCacheFill = fakeFill(&fills)

		inst := t.TempDir()
		d, err := fcPrepareDisk(inst, 0, DiskConfig{Path: top}, true, 7, 200007)
		if err != nil {
			t.Fatal(err)
		}

		dst := filepath.Join(inst, "disk-0.raw")
		want := [][]string{
			fcCmd("qemu-img", "info", "--output=json", "--backing-chain", fcResolve(top)),
			fcCmd("qemu-img", "convert", "-O", "raw", fcResolve(top), dst),
		}
		if !reflect.DeepEqual(cmds.get(), want) {
			t.Errorf("commands =\n%q\nwant\n%q", cmds.get(), want)
		}
		if fills != 0 {
			t.Errorf("fills = %v, want 0", fills)
		}
		if d.HostPath != dst {
			t.Errorf("disk = %+v", d)
		}
	})

	t.Run("snapshot false", func(t *testing.T) {
		src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.raw"), []byte("raw"))
		cmds := setFCDiskSeams(t, nil)
		fcRunCmd = func(args ...string) (string, error) {
			cmds.add(args)
			return fcDiskReply(info(t, src, "raw"))(args)
		}

		d, err := fcPrepareDisk(t.TempDir(), 0, DiskConfig{Path: src}, false, 7, 200007)
		if err != nil {
			t.Fatal(err)
		}

		want := [][]string{
			fcCmd("qemu-img", "info", "--output=json", "--backing-chain", fcResolve(src)),
			fcCmd("setfacl", "-m", "u:200007:rw", fcResolve(src)),
		}
		if !reflect.DeepEqual(cmds.get(), want) {
			t.Errorf("commands =\n%q\nwant\n%q", cmds.get(), want)
		}
		if d.HostPath != fcResolve(src) || d.IsBlock {
			t.Errorf("disk = %+v", d)
		}

		cmds.reset()
		if err := fcReleaseDisk(d); err != nil {
			t.Fatal(err)
		}
		if want := [][]string{fcCmd("setfacl", "-x", "u:200007", fcResolve(src))}; !reflect.DeepEqual(cmds.get(), want) {
			t.Errorf("release commands = %q, want %q", cmds.get(), want)
		}
	})

	t.Run("snapshot false qcow2", func(t *testing.T) {
		src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.qcow2"), []byte("qcow2"))
		setFCDiskSeams(t, func(args []string) (string, error) {
			return fcDiskReply(info(t, src, "qcow2"))(args)
		})

		_, err := fcPrepareDisk(t.TempDir(), 0, DiskConfig{Path: src}, false, 7, 200007)
		if want := "firecracker needs a raw image when snapshot is false: " + src; err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
	})

	t.Run("device-mapper failure", func(t *testing.T) {
		src := fcWriteFile(t, filepath.Join(t.TempDir(), "a.raw"), []byte("raw"))
		cmds := setFCDiskSeams(t, nil)
		fcRunCmd = func(args ...string) (string, error) {
			cmds.add(args)
			if args[0] == "dmsetup" && args[1] == "create" {
				return "no dm", fmt.Errorf("exit status 1")
			}
			return fcDiskReply(info(t, src, "raw"))(args)
		}

		var fills int32
		fcCacheFill = fakeFill(&fills)
		fcDMUsable = func() bool { return true }

		if _, err := fcPrepareDisk(t.TempDir(), 0, DiskConfig{Path: src}, true, 7, 200007); err == nil {
			t.Fatal("want error")
		}

		calls := cmds.get()
		want := [][]string{
			fcCmd("losetup", "-d", "/dev/loop4"),
			fcCmd("losetup", "-d", "/dev/loop3"),
		}
		if len(calls) < 2 || !reflect.DeepEqual(calls[len(calls)-2:], want) {
			t.Errorf("commands = %q, want cleanup %q", calls, want)
		}

		id, _ := fcStatSource(src)
		if got := fcCache.inUse(fcCacheKey(id)); got != 0 {
			t.Errorf("inUse() = %v, want 0", got)
		}
	})
}

func TestFCPrepareDiskReadOnly(t *testing.T) {
	for _, format := range []string{"raw", "qcow2"} {
		t.Run(format, func(t *testing.T) {
			src := fcWriteFile(t, filepath.Join(t.TempDir(), "rootfs."+format), []byte("image"))
			info := fcImageJSON(t, fcImage{Filename: src, Format: format, VirtualSize: 1 << 20})
			cmds := setFCDiskSeams(t, func(args []string) (string, error) {
				if args[0] == "qemu-img" && args[1] == "info" {
					return info, nil
				}
				return "", nil
			})

			var fills []string
			fcCacheFill = func(src, dst, format string) error {
				fills = append(fills, format)
				return os.WriteFile(dst, []byte("image"), 0600)
			}
			fcReflinkOK = func() bool { t.Error("read-only disk checked reflink"); return true }
			fcDMUsable = func() bool { t.Error("read-only disk checked device-mapper"); return true }
			fcCloneFile = func(string, string) error { t.Error("read-only disk made a clone"); return nil }
			var chowns int
			fcChown = func(string, int, int) error { chowns++; return nil }

			inst1, inst2 := t.TempDir(), t.TempDir()
			dc := DiskConfig{Path: src, ReadOnly: true}

			// The snapshot setting does not apply to a read-only disk.
			d1, err := fcPrepareDisk(inst1, 0, dc, false, 7, 200007)
			if err != nil {
				t.Fatal(err)
			}
			d2, err := fcPrepareDisk(inst2, 0, dc, true, 8, 200008)
			if err != nil {
				t.Fatal(err)
			}

			id, _ := fcStatSource(src)
			key := fcCacheKey(id)
			cached := filepath.Join(fcCacheDir(), key+".raw")
			infoCmd := fcCmd("qemu-img", "info", "--output=json", "--backing-chain", fcResolve(src))

			if want := [][]string{infoCmd}; !reflect.DeepEqual(cmds.get(), want) {
				t.Errorf("commands =\n%q\nwant\n%q", cmds.get(), want)
			}
			for i, d := range []fcDisk{d1, d2} {
				if d.HostPath != cached || !d.ReadOnly || d.IsBlock || d.Scratch || d.CacheType != "Writeback" {
					t.Errorf("disk %d = %+v, want the cache entry %v, read-only", i, d, cached)
				}
			}
			if fi, err := os.Stat(cached); err != nil || fi.Mode().Perm() != 0444 {
				t.Errorf("cache entry: %v, %v, want mode 0444", fi, err)
			}
			if !reflect.DeepEqual(fills, []string{format}) {
				t.Errorf("fills = %q, want [%v]", fills, format)
			}
			if chowns != 0 {
				t.Errorf("chown calls = %d, want 0", chowns)
			}
			for _, inst := range []string{inst1, inst2} {
				if entries, _ := os.ReadDir(inst); len(entries) != 0 {
					t.Errorf("instance directory %v has %d files, want 0", inst, len(entries))
				}
			}
			if got := fcCache.inUse(key); got != 2 {
				t.Errorf("inUse() = %v, want 2", got)
			}

			cmds.reset()
			for _, d := range []fcDisk{d1, d2} {
				if err := fcReleaseDisk(d); err != nil {
					t.Fatal(err)
				}
			}
			if len(cmds.get()) != 0 {
				t.Errorf("release commands = %q, want none", cmds.get())
			}
			if got := fcCache.inUse(key); got != 0 {
				t.Errorf("inUse() after release = %v, want 0", got)
			}
		})
	}
}

func TestFCPrepareDiskReadOnlyChain(t *testing.T) {
	dir := t.TempDir()
	src := fcWriteFile(t, filepath.Join(dir, "ov.qcow2"), []byte("overlay"))
	base := fcWriteFile(t, filepath.Join(dir, "base.raw"), []byte("base"))
	info := fcImageJSON(t,
		fcImage{Filename: src, Format: "qcow2", BackingFilename: base, VirtualSize: 1 << 20},
		fcImage{Filename: base, Format: "raw", VirtualSize: 1 << 20},
	)
	setFCDiskSeams(t, func(args []string) (string, error) {
		if args[0] == "qemu-img" && args[1] == "info" {
			return info, nil
		}
		return "", nil
	})
	fcCacheFill = func(string, string, string) error {
		t.Error("ro disk with a backing chain filled the cache")
		return nil
	}

	_, err := fcPrepareDisk(t.TempDir(), 0, DiskConfig{Path: src, ReadOnly: true}, true, 7, 200007)
	if err == nil || !strings.Contains(err.Error(), "without a backing file") {
		t.Fatalf("fcPrepareDisk() = %v, want a backing file error", err)
	}
}

func TestFCCacheClearOrphans(t *testing.T) {
	setFCDiskSeams(t, nil)

	dir := fcCacheDir()
	for _, name := range []string{
		"aaaa.raw", "aaaa.json",
		"bbbb.raw.tmp-1", "bbbb.json.tmp-1",
		"cccc.json",
		"dddd.raw.tmp-2",
	} {
		fcWriteFile(t, filepath.Join(dir, name), []byte("x"))
	}
	fcCache.inflight["dddd"] = &fcInflight{done: make(chan struct{})}

	n, err := fcCacheClear()
	if err != nil || n != 1 {
		t.Fatalf("fcCacheClear() = %v, %v, want 1 entry", n, err)
	}

	var names []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{"dddd.raw.tmp-2"}; !reflect.DeepEqual(names, want) {
		t.Errorf("cache directory has %q, want %q", names, want)
	}
}

func TestFCPrepareScratch(t *testing.T) {
	setFCDiskSeams(t, nil)

	var chowns []string
	fcChown = func(p string, uid, gid int) error {
		chowns = append(chowns, fmt.Sprintf("%v:%d:%d", p, uid, gid))
		return nil
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "scratch.raw")

	d, err := fcPrepareScratch(dir, 64<<20, 200007)
	if err != nil {
		t.Fatal(err)
	}
	if d.HostPath != p || !d.Scratch || d.ReadOnly || d.IsBlock || d.CacheType != "Writeback" {
		t.Errorf("scratch disk = %+v", d)
	}
	if want := []string{p + ":200007:200007"}; !reflect.DeepEqual(chowns, want) {
		t.Errorf("chowns = %q, want %q", chowns, want)
	}

	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 64<<20 {
		t.Errorf("size = %d, want %d", fi.Size(), 64<<20)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Blocks*512 >= 1<<20 {
		t.Errorf("scratch disk uses %d bytes, want a sparse file", st.Blocks*512)
	}

	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("guest data"), 4096); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// A new scratch disk is empty.
	if _, err := fcPrepareScratch(dir, 1<<20, 200007); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1<<20 || !fcAllZero(data) {
		t.Errorf("new scratch disk has %d bytes, all zero %v", len(data), fcAllZero(data))
	}

	if got := fcDriveID(1, d); got != "scratch" {
		t.Errorf("fcDriveID(scratch) = %q", got)
	}
	if got := fcDriveID(1, fcDisk{}); got != "disk1" {
		t.Errorf("fcDriveID(disk 1) = %q", got)
	}
}

func TestFCParseSize(t *testing.T) {
	for in, want := range map[string]int64{
		"":     0,
		"0":    0,
		"512":  512,
		"4k":   4 << 10,
		"64M":  64 << 20,
		"1G":   1 << 30,
		"2T":   2 << 40,
		"100m": 100 << 20,
	} {
		if got, err := fcParseSize(in); err != nil || got != want {
			t.Errorf("fcParseSize(%q) = %v, %v, want %v", in, got, err, want)
		}
		if err := validFCScratch(NewVMConfig(), in); err != nil {
			t.Errorf("validFCScratch(%q) = %v", in, err)
		}
	}

	for _, in := range []string{"M", "abc", "-1M", "1.5G", "64MB", "100", "9999999999T"} {
		if got, err := fcParseSize(in); err == nil {
			t.Errorf("fcParseSize(%q) = %v, want error", in, got)
		}
		if err := validFCScratch(NewVMConfig(), in); err == nil {
			t.Errorf("validFCScratch(%q) = nil, want error", in)
		}
	}
}

func TestFCCheckDiskOptions(t *testing.T) {
	plain := DiskConfigs{{Path: "/img/a.qcow2", Interface: "virtio", Cache: "unsafe"}}
	ro := DiskConfigs{{Path: "/img/a.qcow2"}, {Path: "/img/root.squashfs", ReadOnly: true}}
	want := "disk option ro is supported only for firecracker VMs"

	for _, vmType := range []VMType{KVM, CONTAINER, ANDROID, FIRECRACKER} {
		if err := fcCheckDiskOptions(vmType, plain); err != nil {
			t.Errorf("fcCheckDiskOptions(%v, plain) = %v", vmType, err)
		}
	}

	for _, vmType := range []VMType{KVM, CONTAINER, ANDROID} {
		cfg := NewVMConfig()
		cfg.Disks = ro
		if _, err := NewVM("ro1", DefaultNamespace, vmType, cfg); err == nil || err.Error() != want {
			t.Errorf("NewVM(%v) error = %v, want %q", vmType, err, want)
		}
	}

	cfg := NewVMConfig()
	cfg.Disks = ro
	vm, err := NewVM("ro1", DefaultNamespace, FIRECRACKER, cfg)
	if err != nil {
		t.Fatalf("NewVM(firecracker) = %v", err)
	}
	// NewVM returns the VM locked, so read the field directly.
	if got := vm.(*FirecrackerVM).Disks.String(); got != "/img/a.qcow2 /img/root.squashfs,ro" {
		t.Errorf("disks = %q, want /img/a.qcow2 /img/root.squashfs,ro", got)
	}
}

func TestCLIDiskCache(t *testing.T) {
	setFCDiskSeams(t, nil)

	dir := fcCacheDir()
	created := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	for _, e := range []struct {
		key, source, format string
		size                int
	}{
		{"1111111111111111", "/img/a.raw", "raw", 1024},
		{"2222222222222222", "/img/b.qcow2", "qcow2", 2048},
	} {
		fcWriteFile(t, filepath.Join(dir, e.key+".raw"), make([]byte, e.size))
		meta, _ := json.Marshal(fcCacheMeta{Source: e.source, Format: e.format, Created: created})
		fcWriteFile(t, filepath.Join(dir, e.key+".json"), meta)
	}
	fcWriteFile(t, filepath.Join(dir, "3333333333333333.raw"), nil)
	fcWriteFile(t, filepath.Join(dir, "4444444444444444.raw.tmp-abc"), nil)

	fcCache.users["1111111111111111"] = map[int]int{7: 1, 8: 2}
	fcCache.bases["2222222222222222"] = &fcBaseLoop{dev: "/dev/loop3", refs: 1}

	resp := &minicli.Response{}
	if err := cliDiskCache(nil, &minicli.Command{BoolArgs: map[string]bool{}}, resp); err != nil {
		t.Fatal(err)
	}

	if want := []string{"key", "source", "format", "size", "created", "inuse"}; !reflect.DeepEqual(resp.Header, want) {
		t.Errorf("header = %q, want %q", resp.Header, want)
	}

	if len(resp.Tabular) != 3 {
		t.Fatalf("rows = %q, want 3", resp.Tabular)
	}
	if want := []string{"1111111111111111", "/img/a.raw", "raw", "1.0 KiB", "2026-11-05T12:00:00Z", "2"}; !reflect.DeepEqual(resp.Tabular[0], want) {
		t.Errorf("row 0 = %q, want %q", resp.Tabular[0], want)
	}
	if want := []string{"2222222222222222", "/img/b.qcow2", "qcow2", "2.0 KiB", "2026-11-05T12:00:00Z", "0"}; !reflect.DeepEqual(resp.Tabular[1], want) {
		t.Errorf("row 1 = %q, want %q", resp.Tabular[1], want)
	}
	if got := resp.Tabular[2]; got[0] != "3333333333333333" || got[1] != "" || got[5] != "0" {
		t.Errorf("row 2 = %q", got)
	}

	resp = &minicli.Response{}
	if err := cliDiskCache(nil, &minicli.Command{BoolArgs: map[string]bool{"clear": true}}, resp); err != nil {
		t.Fatal(err)
	}
	if resp.Response != "1" {
		t.Errorf("clear response = %q, want 1", resp.Response)
	}

	for name, exists := range map[string]bool{
		"1111111111111111.raw":         true,
		"1111111111111111.json":        true,
		"2222222222222222.raw":         true,
		"3333333333333333.raw":         false,
		"4444444444444444.raw.tmp-abc": false,
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != exists {
			t.Errorf("%v exists = %v, want %v", name, err == nil, exists)
		}
	}

	// Without a cache directory, the table is empty.
	*f_firecrackerDir = filepath.Join(t.TempDir(), "missing")
	resp = &minicli.Response{}
	if err := cliDiskCache(nil, &minicli.Command{BoolArgs: map[string]bool{}}, resp); err != nil || len(resp.Tabular) != 0 {
		t.Errorf("missing cache: rows %q, error %v", resp.Tabular, err)
	}
}
