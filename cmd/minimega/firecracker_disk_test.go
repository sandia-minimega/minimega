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
			infoCmd,
			fcCmd("losetup", "--find", "--show", cow2),
			fcCmd("dmsetup", "create", "mm-fc-a1b2c3-8-1", "--table", "0 2048 snapshot /dev/loop3 /dev/loop4 P 8"),
		}
		if !reflect.DeepEqual(cmds.get(), want) {
			t.Errorf("commands =\n%q\nwant\n%q", cmds.get(), want)
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
		"4444444444444444.raw.tmp-abc": true,
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
