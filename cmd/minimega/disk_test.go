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

func TestWritableMountOptions(t *testing.T) {
	cases := []struct {
		name    string
		options []string
		want    [][]string
	}{
		{
			name: "no options",
			want: [][]string{
				{"-w"},
				{"-t", "ntfs-3g"},
			},
		},
		{
			name:    "type and offset",
			options: []string{"-t", "fat", "-o", "offset=100"},
			want: [][]string{
				{"-t", "fat", "-o", "offset=100"},
				{"-t", "ntfs-3g"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := writableMountOptions(c.options); !reflect.DeepEqual(got, c.want) {
				t.Errorf("candidates = %v, want %v", got, c.want)
			}
		})
	}
}

func TestReadOnlyMountOptions(t *testing.T) {
	cases := []struct {
		name    string
		options []string
		want    [][]string
	}{
		{
			name: "no options",
			want: [][]string{
				{"-r"},
				{"-o", "noload", "-r"},
				{"-o", "norecovery", "-r"},
				{"-t", "ntfs-3g", "-o", "force", "-r"},
			},
		},
		{
			name:    "type and offset",
			options: []string{"-t", "fat", "-o", "offset=100"},
			want: [][]string{
				{"-t", "fat", "-o", "offset=100", "-r"},
				{"-t", "fat", "-o", "offset=100", "-o", "noload", "-r"},
				{"-t", "fat", "-o", "offset=100", "-o", "norecovery", "-r"},
			},
		},
		{
			name:    "user requests read-write",
			options: []string{"-w", "-o", "rw"},
			want: [][]string{
				{"-w", "-o", "rw", "-r"},
				{"-w", "-o", "rw", "-o", "noload", "-r"},
				{"-w", "-o", "rw", "-o", "norecovery", "-r"},
				{"-w", "-o", "rw", "-t", "ntfs-3g", "-o", "force", "-r"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			candidates := readOnlyMountOptions(c.options)

			if !reflect.DeepEqual(candidates, c.want) {
				t.Errorf("candidates = %v, want %v", candidates, c.want)
			}

			// -r must be last to override user read-write options
			for _, opts := range candidates {
				if opts[len(opts)-1] != "-r" {
					t.Errorf("candidate %v does not end with -r", opts)
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
		{[]string{"-vt", "ntfs3"}, true},
		{[]string{"-vtntfs3"}, true},
		{[]string{"-ostuff,t"}, false},
		{[]string{"-Ltest"}, false},
		{[]string{"-Mt", "ext4"}, true},
		{[]string{"-p3t"}, false},
	}

	for _, c := range cases {
		if got := hasMountType(c.options); got != c.want {
			t.Errorf("hasMountType(%v) = %v, want %v", c.options, got, c.want)
		}
	}
}

func TestSelectPartition(t *testing.T) {
	cases := []struct {
		name      string
		files     []string
		partition string
		want      string
		wantErr   string
	}{
		{name: "none", files: []string{"nbd0p1"}, partition: "none", want: "nbd0"},
		{name: "default", files: []string{"nbd0p1", "nbd1p2", "nbd0p1x"}, want: "nbd0p1"},
		{name: "default with gap", files: []string{"nbd0p1", "nbd0p3"}, wantErr: "multiple found"},
		{name: "only partition 2", files: []string{"nbd0p2"}, partition: "2", want: "nbd0p2"},
		{name: "default only partition 2", files: []string{"nbd0p2"}, want: "nbd0p2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()

			// regular files stand in for device files; the rescan ioctl fails on them
			for _, name := range append(c.files, "nbd0") {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got, err := selectPartition(filepath.Join(dir, "nbd0"), c.partition)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("selectPartition error = %v, want it to contain %q", err, c.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("selectPartition: %v", err)
			}
			if want := filepath.Join(dir, c.want); got != want {
				t.Errorf("selectPartition = %q, want %q", got, want)
			}
		})
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

// makeGuestTree creates files, symlinks, and a FIFO under root.
func makeGuestTree(t *testing.T, root string) {
	t.Helper()

	for _, dir := range []string{"etc", "data/sub", "var", "special"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		"etc/hostname":   "guest\n",
		"data/a.txt":     "a\n",
		"data/sub/b.txt": "b\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.Symlink("/etc/hostname", filepath.Join(root, "abs-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../../..", filepath.Join(root, "var/up")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "special/fifo"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

func TestExtractFiles(t *testing.T) {
	root := t.TempDir()
	makeGuestTree(t, root)

	// extract must not write through this host symlink
	host := t.TempDir()
	sentinel := filepath.Join(host, "sentinel")
	if err := os.WriteFile(sentinel, []byte("host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, filepath.Join(host, "existing-link")); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(host, "out")
	pairs := map[string]string{
		filepath.Join(out, "hostname"):          "/etc/hostname",
		filepath.Join(out, "data"):              "/data",
		filepath.Join(out, "abs-link"):          "/abs-link",
		filepath.Join(out, "climb"):             "/var/up/etc/hostname",
		filepath.Join(host, "existing-link"):    "data/a.txt",
		filepath.Join(out, "nested/dir/copied"): "/../data/sub/b.txt",
	}

	if err := extractFiles(root, pairs); err != nil {
		t.Fatalf("extractFiles: %v", err)
	}

	if got := readFile(t, filepath.Join(out, "hostname")); got != "guest\n" {
		t.Errorf("hostname = %q, want %q", got, "guest\n")
	}
	if got := readFile(t, filepath.Join(out, "data/sub/b.txt")); got != "b\n" {
		t.Errorf("data/sub/b.txt = %q, want %q", got, "b\n")
	}
	if got := readFile(t, filepath.Join(out, "climb")); got != "guest\n" {
		t.Errorf("symlink above root was not resolved inside the image: got %q", got)
	}
	if got := readFile(t, filepath.Join(out, "nested/dir/copied")); got != "b\n" {
		t.Errorf("nested/dir/copied = %q, want %q", got, "b\n")
	}

	if target, err := os.Readlink(filepath.Join(out, "abs-link")); err != nil || target != "/etc/hostname" {
		t.Errorf("abs-link = %q, %v; want a symlink to /etc/hostname", target, err)
	}

	if got := readFile(t, sentinel); got != "host\n" {
		t.Errorf("sentinel was overwritten through a symlink: %q", got)
	}
	if info, err := os.Lstat(filepath.Join(host, "existing-link")); err != nil || !info.Mode().IsRegular() {
		t.Errorf("existing-link should now be a regular file: %v, %v", info, err)
	}
}

func TestExtractFilesDashDestination(t *testing.T) {
	root := t.TempDir()
	makeGuestTree(t, root)

	t.Chdir(t.TempDir())

	if err := extractFiles(root, map[string]string{"--help": "/etc/hostname"}); err != nil {
		t.Fatalf("extractFiles: %v", err)
	}

	if got := readFile(t, "--help"); got != "guest\n" {
		t.Errorf("--help = %q, want %q", got, "guest\n")
	}
}

func TestExtractFilesErrors(t *testing.T) {
	root := t.TempDir()
	makeGuestTree(t, root)

	cases := []struct {
		name   string
		source string
		want   string
	}{
		{name: "missing source", source: "/nope", want: "error accessing '/nope'"},
		{name: "empty source", source: "", want: "empty source path"},
		{name: "fifo", source: "/special/fifo", want: "'/special/fifo' is a special file"},
		{name: "directory with fifo", source: "/special", want: "'/special/fifo' is a special file"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := t.TempDir()

			// "a" is valid, so nothing copied also shows sources are checked first
			pairs := map[string]string{
				filepath.Join(out, "a"): "/etc/hostname",
				filepath.Join(out, "b"): c.source,
			}

			err := extractFiles(root, pairs)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("extractFiles error = %v, want it to contain %q", err, c.want)
			}

			if entries, _ := os.ReadDir(out); len(entries) != 0 {
				t.Errorf("files were copied despite the error: %v", entries)
			}
		})
	}
}

func TestMountedImageClose(t *testing.T) {
	cases := []struct {
		name       string
		busy       int // unmount calls that return EBUSY
		wantErr    string
		disconnect bool
	}{
		{name: "busy once", busy: 1, disconnect: true},
		{name: "always busy", busy: 100, wantErr: "stays connected"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			oldUnmount, oldDisconnect := unmount, disconnectDevice
			t.Cleanup(func() { unmount, disconnectDevice = oldUnmount, oldDisconnect })

			calls := 0
			unmount = func(string, int) error {
				calls++
				if calls <= c.busy {
					return syscall.EBUSY
				}
				return nil
			}

			disconnected := false
			disconnectDevice = func(string) error {
				disconnected = true
				return nil
			}

			mi := &mountedImage{nbdPath: "/dev/nbd0", mntDir: t.TempDir(), mounted: true}
			err := mi.Close()

			if c.wantErr == "" && err != nil {
				t.Fatalf("Close: %v", err)
			}
			if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
				t.Fatalf("Close error = %v, want it to contain %q", err, c.wantErr)
			}
			if disconnected != c.disconnect {
				t.Errorf("disconnected = %v, want %v", disconnected, c.disconnect)
			}
			_, statErr := os.Stat(mi.mntDir)
			if exists := statErr == nil; exists == c.disconnect {
				t.Errorf("mount point exists = %v, want %v", exists, !c.disconnect)
			}
		})
	}
}
