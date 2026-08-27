// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func TestParseInjectPairs(t *testing.T) {
	cases := []struct {
		name    string
		in      []string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "single pair",
			in:   []string{"src:dst"},
			want: map[string]string{"dst": "src"},
		},
		{
			name: "multiple pairs",
			in:   []string{"src1:dst1", "src2:dst2"},
			want: map[string]string{"dst1": "src1", "dst2": "src2"},
		},
		{
			name:    "malformed - no colon",
			in:      []string{"srcdst"},
			wantErr: true,
		},
		{
			name:    "malformed - too many colons",
			in:      []string{"src:dst:extra"},
			wantErr: true,
		},
		{
			name:    "duplicate destination",
			in:      []string{"src1:dst", "src2:dst"},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseInjectPairs(c.in)

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("pairs = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseFiles(t *testing.T) {
	cases := []struct {
		name      string
		files     interface{}
		delete    bool
		wantPairs map[string]string
		wantPaths []string
		wantErr   bool
	}{
		{
			name:      "inject list arg",
			files:     []string{"src1:dst1", "src2:dst2"},
			wantPairs: map[string]string{"dst1": "src1", "dst2": "src2"},
		},
		{
			name:      "inject string arg (options case)",
			files:     "src:dst",
			wantPairs: map[string]string{"dst": "src"},
		},
		{
			name:      "delete single path from list arg",
			delete:    true,
			files:     []string{"foo.txt"},
			wantPaths: []string{"foo.txt"},
		},
		{
			name:      "delete comma-separated paths from list arg",
			delete:    true,
			files:     []string{"foo.txt,bar/baz.txt"},
			wantPaths: []string{"foo.txt", "bar/baz.txt"},
		},
		{
			name:      "delete single path from string arg",
			delete:    true,
			files:     "foo.txt",
			wantPaths: []string{"foo.txt"},
		},
		{
			name:      "delete comma-separated paths from string arg",
			delete:    true,
			files:     "foo.txt,bar/baz.txt",
			wantPaths: []string{"foo.txt", "bar/baz.txt"},
		},
		{
			name:    "malformed inject pair",
			files:   []string{"nodest"},
			wantErr: true,
		},
		{
			name:    "unknown type",
			files:   42,
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pairs, paths, err := parseFiles(c.files, c.delete)

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantPairs != nil && !reflect.DeepEqual(pairs, c.wantPairs) {
				t.Errorf("pairs = %v, want %v", pairs, c.wantPairs)
			}
			if c.wantPaths != nil && !reflect.DeepEqual(paths, c.wantPaths) {
				t.Errorf("paths = %v, want %v", paths, c.wantPaths)
			}
		})
	}
}

func TestWritableMountArgsNoOptions(t *testing.T) {
	candidates := writableMountArgs(nil)("/dev/nbd0p1")

	want := [][]string{
		{"mount", "-w", "/dev/nbd0p1"},
		{"mount", "-t", "ntfs-3g", "/dev/nbd0p1"},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Errorf("candidates = %v, want %v", candidates, want)
	}
}

func TestWritableMountArgsWithOptions(t *testing.T) {
	options := []string{"-t", "fat", "-o", "offset=100"}
	candidates := writableMountArgs(options)("/dev/nbd0p1")

	want := [][]string{
		{"mount", "-t", "fat", "-o", "offset=100", "/dev/nbd0p1"},
		{"mount", "-t", "ntfs-3g", "/dev/nbd0p1"},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Errorf("candidates = %v, want %v", candidates, want)
	}
}

func TestReadOnlyMountArgsNoOptions(t *testing.T) {
	candidates := readOnlyMountArgs(nil)("/dev/nbd0p1")

	want := [][]string{
		{"mount", "-r", "/dev/nbd0p1"},
		{"mount", "-o", "noload", "-r", "/dev/nbd0p1"},
		{"mount", "-o", "norecovery", "-r", "/dev/nbd0p1"},
		{"mount", "-t", "ntfs-3g", "-o", "force", "-r", "/dev/nbd0p1"},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Errorf("candidates = %v, want %v", candidates, want)
	}
}

func TestReadOnlyMountArgsWithOptions(t *testing.T) {
	cases := []struct {
		name    string
		options []string
		want    [][]string
	}{
		{
			name:    "type and offset",
			options: []string{"-t", "fat", "-o", "offset=100"},
			want: [][]string{
				{"mount", "-t", "fat", "-o", "offset=100", "-r", "/dev/nbd0p1"},
				{"mount", "-t", "fat", "-o", "offset=100", "-o", "noload", "-r", "/dev/nbd0p1"},
				{"mount", "-t", "fat", "-o", "offset=100", "-o", "norecovery", "-r", "/dev/nbd0p1"},
			},
		},
		{
			name:    "user requests read-write",
			options: []string{"-w", "-o", "rw"},
			want: [][]string{
				{"mount", "-w", "-o", "rw", "-r", "/dev/nbd0p1"},
				{"mount", "-w", "-o", "rw", "-o", "noload", "-r", "/dev/nbd0p1"},
				{"mount", "-w", "-o", "rw", "-o", "norecovery", "-r", "/dev/nbd0p1"},
				{"mount", "-w", "-o", "rw", "-t", "ntfs-3g", "-o", "force", "-r", "/dev/nbd0p1"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			candidates := readOnlyMountArgs(c.options)("/dev/nbd0p1")

			if !reflect.DeepEqual(candidates, c.want) {
				t.Errorf("candidates = %v, want %v", candidates, c.want)
			}

			// read-only must be the last option so that it overrides any
			// user-supplied read-write option
			for _, args := range candidates {
				if args[len(args)-2] != "-r" {
					t.Errorf("candidate %v does not end with -r", args)
				}
			}
		})
	}
}

func TestHasMountType(t *testing.T) {
	cases := []struct {
		options []string
		want    bool
	}{
		{nil, false},
		{[]string{"-o", "offset=100"}, false},
		{[]string{"-t", "xfs"}, true},
		{[]string{"-text4"}, true},
		{[]string{"--types", "xfs"}, true},
		{[]string{"--types=xfs"}, true},
	}

	for _, c := range cases {
		if got := hasMountType(c.options); got != c.want {
			t.Errorf("hasMountType(%v) = %v, want %v", c.options, got, c.want)
		}
	}
}

func TestResolveInRoot(t *testing.T) {
	root := t.TempDir()

	mustMkdir := func(p string) {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustSymlink := func(target, p string) {
		if err := os.Symlink(target, filepath.Join(root, p)); err != nil {
			t.Fatal(err)
		}
	}

	mustMkdir("etc")
	mustMkdir("var")
	mustMkdir("run")
	mustSymlink("/run", "var/run")
	mustSymlink("../../../../..", "var/up")
	mustSymlink("/etc/passwd", "var/abs-file")
	mustSymlink("/does/not/exist", "var/dangling")
	mustSymlink("loop2", "var/loop1")
	mustSymlink("loop1", "var/loop2")
	if err := os.WriteFile(filepath.Join(root, "etc/passwd"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "plain", path: "/etc/passwd", want: "/etc/passwd"},
		{name: "relative", path: "etc/passwd", want: "/etc/passwd"},
		{name: "root", path: "/", want: "/"},
		{name: "parent traversal", path: "/../../etc/passwd", want: "/etc/passwd"},
		{name: "parent in middle", path: "/var/../../etc", want: "/etc"},
		{name: "absolute dir symlink", path: "/var/run/foo", want: "/run/foo"},
		{name: "relative escape symlink", path: "/var/up/etc", want: "/etc"},
		{name: "final symlink not followed", path: "/var/abs-file", want: "/var/abs-file"},
		{name: "dangling final symlink", path: "/var/dangling", want: "/var/dangling"},
		{name: "trailing slash follows final symlink", path: "/var/run/", want: "/run"},
		{name: "trailing slash on dir", path: "/etc/", want: "/etc"},
		{name: "dot after file", path: "/etc/passwd/.", wantErr: true},
		{name: "dot-dot after file", path: "/etc/passwd/..", wantErr: true},
		{name: "trailing slash on dangling symlink", path: "/var/dangling/", wantErr: true},
		{name: "symlink loop", path: "/var/loop1/x", wantErr: true},
		{name: "missing parent", path: "/nope/x", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveInRoot(root, c.path)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if want := filepath.Join(root, c.want); got != want {
				t.Errorf("resolveInRoot(%q) = %q, want %q", c.path, got, want)
			}
		})
	}
}

func TestCheckExtractable(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "ok/sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok/sub/file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/null", filepath.Join(root, "ok/link")); err != nil {
		t.Fatal(err)
	}

	if err := checkExtractable(root, filepath.Join(root, "ok")); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "bad/sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "bad/sub/fifo"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := checkExtractable(root, filepath.Join(root, "bad"))
	if err == nil || !strings.Contains(err.Error(), "'/bad/sub/fifo'") {
		t.Errorf("expected special file error for /bad/sub/fifo, got %v", err)
	}
}
