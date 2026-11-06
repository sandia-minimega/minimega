// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type fcFakeFileInfo struct {
	name string
	mode os.FileMode
}

func (f fcFakeFileInfo) Name() string {
	return f.name
}

func (f fcFakeFileInfo) Size() int64 {
	return 0
}

func (f fcFakeFileInfo) Mode() os.FileMode {
	return f.mode
}

func (f fcFakeFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (f fcFakeFileInfo) IsDir() bool {
	return f.mode.IsDir()
}

func (f fcFakeFileInfo) Sys() any {
	return nil
}

// fcFakeStat returns mode 0755 for every path, except the paths in modes.
func fcFakeStat(modes map[string]os.FileMode) func(string) (os.FileInfo, error) {
	return func(p string) (os.FileInfo, error) {
		if m, ok := modes[p]; ok {
			if m == 0 {
				return nil, os.ErrNotExist
			}
			return fcFakeFileInfo{name: filepath.Base(p), mode: m}, nil
		}
		return fcFakeFileInfo{name: filepath.Base(p), mode: os.ModeDir | 0o755}, nil
	}
}

// testELF returns a minimal x86_64 ELF file with one program header of each
// given type.
func testELF(t *testing.T, progs ...elf.ProgType) []byte {
	t.Helper()

	hdr := elf.Header64{
		Type:      uint16(elf.ET_EXEC),
		Machine:   uint16(elf.EM_X86_64),
		Version:   uint32(elf.EV_CURRENT),
		Phoff:     64,
		Ehsize:    64,
		Phentsize: 56,
		Phnum:     uint16(len(progs)),
	}
	copy(hdr.Ident[:], elf.ELFMAG)
	hdr.Ident[elf.EI_CLASS] = byte(elf.ELFCLASS64)
	hdr.Ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	hdr.Ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, hdr); err != nil {
		t.Fatal(err)
	}
	for _, p := range progs {
		if err := binary.Write(&buf, binary.LittleEndian, elf.Prog64{Type: uint32(p)}); err != nil {
			t.Fatal(err)
		}
	}

	return buf.Bytes()
}

func TestElfIsStatic(t *testing.T) {
	static, err := elfIsStatic(bytes.NewReader(testELF(t, elf.PT_LOAD)))
	if err != nil || !static {
		t.Errorf("static ELF: got %v, %v; want true, nil", static, err)
	}

	static, err = elfIsStatic(bytes.NewReader(testELF(t, elf.PT_LOAD, elf.PT_INTERP)))
	if err != nil || static {
		t.Errorf("dynamic ELF: got %v, %v; want false, nil", static, err)
	}

	if _, err := elfIsStatic(strings.NewReader("#!/bin/sh\necho Firecracker v1.17.0\n")); err == nil {
		t.Error("script: expected an error")
	}
}

func TestParseFCVersion(t *testing.T) {
	cases := []struct {
		in   string
		want fcVersion
		err  bool
	}{
		{in: "Firecracker v1.17.0\n", want: fcVersion{1, 17, 0}},
		{in: "Jailer v1.17.0\n", want: fcVersion{1, 17, 0}},
		{in: "Firecracker v1.18.2-dev\n\nSupported snapshot data format versions: v8.0.0\n", want: fcVersion{1, 18, 2}},
		{in: "Firecracker 1.17\n", err: true},
		{in: "", err: true},
	}

	for _, tc := range cases {
		got, err := parseFCVersion(tc.in)
		if (err != nil) != tc.err {
			t.Errorf("parseFCVersion(%q) error = %v, want error %v", tc.in, err, tc.err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseFCVersion(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	if !(fcVersion{1, 16, 2}).Less(fcMinVersion) || fcMinVersion.Less(fcVersion{1, 17, 0}) || !fcMinVersion.Less(fcVersion{2, 0, 0}) {
		t.Error("fcVersion.Less is wrong")
	}
}

func TestFCCheckBinaries(t *testing.T) {
	versions := map[string]string{}
	var calls atomic.Int32

	oldRun, oldStat := fcRunVersion, fcPermStat
	t.Cleanup(func() { fcRunVersion, fcPermStat = oldRun, oldStat })

	fcRunVersion = func(path string) (string, error) {
		calls.Add(1)
		if v, ok := versions[path]; ok {
			return v, nil
		}
		return "", errors.New("no such binary")
	}
	fcPermStat = fcFakeStat(nil)

	setup := func(t *testing.T, fcVer, jailerVer string, fcProgs ...elf.ProgType) (string, string) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}

		fc := filepath.Join(dir, "firecracker")
		jailer := filepath.Join(dir, "jailer")
		if len(fcProgs) == 0 {
			fcProgs = []elf.ProgType{elf.PT_LOAD}
		}
		if err := os.WriteFile(fc, testELF(t, fcProgs...), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(jailer, testELF(t, elf.PT_LOAD), 0o755); err != nil {
			t.Fatal(err)
		}

		versions[fc] = "Firecracker " + fcVer + "\n"
		versions[jailer] = "Jailer " + jailerVer + "\n"
		return fc, jailer
	}

	t.Run("ok", func(t *testing.T) {
		fcPath, jailerPath := setup(t, "v1.17.0", "v1.17.0")

		fc, jailer, err := fcCheckBinaries(fcPath, jailerPath)
		if err != nil {
			t.Fatal(err)
		}
		if fc.Version != (fcVersion{1, 17, 0}) || !fc.Static || jailer.Version != fc.Version {
			t.Errorf("got %+v and %+v", fc, jailer)
		}

		n := calls.Load()
		if _, _, err := fcCheckBinaries(fcPath, jailerPath); err != nil {
			t.Fatal(err)
		}
		if calls.Load() != n {
			t.Error("second check ran --version again")
		}
	})

	errCases := []struct {
		name      string
		fcVer     string
		jailerVer string
		progs     []elf.ProgType
		want      string
	}{
		{"old", "v1.16.2", "v1.16.2", nil, "is v1.16.2; minimega needs v1.17.0 or later"},
		{"dynamic", "v1.17.0", "v1.17.0", []elf.ProgType{elf.PT_LOAD, elf.PT_INTERP}, "is not statically linked"},
		{"mismatch", "v1.17.0", "v1.17.1", nil, "must be the same version"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			fcPath, jailerPath := setup(t, tc.fcVer, tc.jailerVer, tc.progs...)

			_, _, err := fcCheckBinaries(fcPath, jailerPath)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error with %q", err, tc.want)
			}
			if tc.name == "old" && !strings.Contains(err.Error(), fcPath) {
				t.Errorf("error %q does not name %v", err, fcPath)
			}
		})
	}

	t.Run("missing jailer", func(t *testing.T) {
		fcPath, jailerPath := setup(t, "v1.17.0", "v1.17.0")
		os.Remove(jailerPath)

		_, jailer, err := fcCheckBinaries(fcPath, jailerPath)
		if err == nil || jailer.Err == nil || !strings.Contains(err.Error(), "jailer") {
			t.Fatalf("got %v, want a jailer error", err)
		}
	})

	t.Run("unsafe permissions", func(t *testing.T) {
		fcPath, jailerPath := setup(t, "v1.17.0", "v1.17.0")
		fcPermStat = fcFakeStat(map[string]os.FileMode{filepath.Dir(fcPath): os.ModeDir | 0o777})
		defer func() { fcPermStat = fcFakeStat(nil) }()

		_, _, err := fcCheckBinaries(fcPath, jailerPath)
		if err == nil || !strings.Contains(err.Error(), filepath.Dir(fcPath)+" is writable by group or others") {
			t.Fatalf("got %v, want a permission error", err)
		}
	})
}

func TestFCUnsafePerms(t *testing.T) {
	stat := fcFakeStat(map[string]os.FileMode{
		"/a":     os.ModeDir | 0o775,
		"/a/b/c": 0o644,
	})

	got := fcUnsafePerms("/a/b/c", stat)
	if len(got) != 1 || !strings.HasPrefix(got[0], "/a is writable") {
		t.Errorf("fcUnsafePerms() = %q, want one problem for /a", got)
	}

	if got := fcUnsafePerms("/x/y", fcFakeStat(nil)); len(got) != 0 {
		t.Errorf("fcUnsafePerms() = %q, want none", got)
	}
}

func TestFCFSTypeName(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{unix.XFS_SUPER_MAGIC, "xfs"},
		{unix.BTRFS_SUPER_MAGIC, "btrfs"},
		{unix.EXT4_SUPER_MAGIC, "ext4"},
		{unix.TMPFS_MAGIC, "tmpfs"},
		{0x1234, "0x1234"},
	}

	for _, tc := range cases {
		if got := fcFSTypeName(tc.in); got != tc.want {
			t.Errorf("fcFSTypeName(%#x) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFCProbeDir(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	statfs := func(_ string, st *unix.Statfs_t) error {
		st.Type = unix.XFS_SUPER_MAGIC
		st.Flags = unix.ST_NODEV | unix.ST_NOEXEC
		return nil
	}
	modes := map[string]os.FileMode{}
	modes[fcNetnsDir] = 0
	modes[filepath.Dir(dir)] = os.ModeDir | 0o777
	stat := fcFakeStat(modes)

	info := fcProbeDir(dir, statfs, stat)
	if !info.Exists || info.FSType != "xfs" || !info.NoDev || !info.NoExec {
		t.Errorf("got %+v", info)
	}

	problems := strings.Join(info.Problems(), "\n")
	for _, want := range []string{"nodev", "noexec", filepath.Dir(dir) + " is writable by group or others"} {
		if !strings.Contains(problems, want) {
			t.Errorf("problems %q do not contain %q", problems, want)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("reflink probe left files: %v", entries)
	}
}

func TestFCProbeDirMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")

	statfs := func(string, *unix.Statfs_t) error {
		t.Error("statfs called for a missing directory")
		return nil
	}

	info := fcProbeDir(dir, statfs, os.Stat)
	if info.Exists || info.Err != nil || len(info.Problems()) != 0 {
		t.Errorf("got %+v", info)
	}
}

func TestFCCheckDirCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fc")

	if info := fcCheckDir(dir); info.Exists {
		t.Fatalf("got %+v before the directory exists", info)
	}

	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	first := fcCheckDir(dir)
	if !first.Exists {
		t.Fatalf("got %+v after the directory exists", first)
	}

	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}

	if second := fcCheckDir(dir); !reflect.DeepEqual(first, second) {
		t.Errorf("second check = %+v, want the cached %+v", second, first)
	}
}

func TestFCIDOverlaps(t *testing.T) {
	passwd := `root:x:0:0:root:/root:/bin/bash
# comment
fc:x:2000000005:100::/:/sbin/nologin
grp:x:100:2000065535::/:/bin/false
edge:x:2000065536:2000065536::/:/bin/false
below:x:1999999999:1::/:/bin/false
bad:x:notanumber:1
short
`
	got := fcIDOverlaps(passwd, 2000000000, 2, 3)
	want := []string{"fc:2000000005", "grp:2000065535"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("passwd: got %q, want %q", got, want)
	}

	group := "root:x:0:\nfc:x:2000000001:alice\nusers:x:100:\n"
	got = fcIDOverlaps(group, 2000000000, 2)
	want = []string{"fc:2000000001"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("group: got %q, want %q", got, want)
	}
}
