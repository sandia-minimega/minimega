// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTarball writes a gzipped tarball to dir/name. Names ending in "/" are
// directories, "name->target" are symlinks, and all other names are files
// whose content is their name.
func writeTarball(t *testing.T, dir, name string, entries ...string) string {
	t.Helper()

	path := filepath.Join(dir, name)

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	for _, e := range entries {
		hdr := &tar.Header{Name: e, Mode: 0755, Typeflag: tar.TypeDir}
		if name, target, ok := strings.Cut(e, "->"); ok {
			hdr = &tar.Header{Name: name, Linkname: target, Mode: 0777, Typeflag: tar.TypeSymlink}
		} else if !strings.HasSuffix(e, "/") {
			hdr = &tar.Header{Name: e, Mode: 0644, Typeflag: tar.TypeReg, Size: int64(len(e))}
		}

		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}

		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e)); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestTarTopLevel(t *testing.T) {
	cases := []struct {
		name    string
		listing string
		want    string
	}{
		{"directory entry", "rootfs/\nrootfs/init\nrootfs/bin/\n", "rootfs"},
		{"no directory entry", "rootfs/init\nrootfs/bin/sh\n", "rootfs"},
		{"dot prefix", "./rootfs/\n./rootfs/init\n", "rootfs"},
		{"absolute names", "/rootfs/\n/rootfs/init\n", "rootfs"},
		{"dot root", "./\n./init\n./bin/\n", ""},
		{"multiple", "rootfs/\nother/\n", ""},
		{"empty", "", ""},
		{"parent", "../\n../init\n", ""},
	}

	for _, c := range cases {
		got, err := tarTopLevel(c.listing)
		if c.want == "" {
			if err == nil {
				t.Errorf("%v: tarTopLevel() = %q, want error", c.name, got)
			}
			continue
		}

		if err != nil || got != c.want {
			t.Errorf("%v: tarTopLevel() = %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestUntar(t *testing.T) {
	dir := t.TempDir()
	tarball := writeTarball(t, dir, "fs.tar.gz", "rootfs/", "rootfs/init", "rootfs/bin/", "rootfs/bin/sh")

	dst, extracted, err := untar(tarball)
	if err != nil || !extracted {
		t.Fatalf("untar: %v, extracted %v", err, extracted)
	}

	if want := filepath.Join(dir, "rootfs"); dst != want {
		t.Errorf("untar() = %q, want %q", dst, want)
	}

	if b, err := os.ReadFile(filepath.Join(dst, "bin", "sh")); err != nil || string(b) != "rootfs/bin/sh" {
		t.Errorf("extracted bin/sh = %q, %v", b, err)
	}

	// the temporary extraction directory should be gone
	if matches, _ := filepath.Glob(filepath.Join(dir, ".untar_*")); len(matches) != 0 {
		t.Errorf("leftover temporary directories: %v", matches)
	}

	// an existing directory is reused rather than extracted again
	marker := filepath.Join(dst, "marker")
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatal(err)
	}

	if dst2, extracted, err := untar(tarball); err != nil || dst2 != dst || extracted {
		t.Errorf("second untar() = %q, %v, %v; want %q, false", dst2, extracted, err, dst)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("second untar replaced the extracted directory: %v", err)
	}
}

func TestUntarRejects(t *testing.T) {
	dir := t.TempDir()

	junk := filepath.Join(dir, "junk.tar.gz")
	if err := os.WriteFile(junk, []byte("junk"), 0644); err != nil {
		t.Fatal(err)
	}

	// the top-level directory name is taken by a file next to the tarball
	clash := writeTarball(t, dir, "clash.tar.gz", "clash/", "clash/init")
	if err := os.WriteFile(filepath.Join(dir, "clash"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"dot root":        writeTarball(t, dir, "dot.tar.gz", "./", "./init"),
		"multiple":        writeTarball(t, dir, "multi.tar.gz", "a/", "a/init", "b/", "b/init"),
		"file at top":     writeTarball(t, dir, "file.tar.gz", "init"),
		"missing tarball": filepath.Join(dir, "missing.tar.gz"),
		"not a tarball":   junk,
		"name clash":      clash,
		"symlink at top":  writeTarball(t, dir, "symlink.tar.gz", "rootfs->/"),
	}

	for name, tarball := range cases {
		if dst, _, err := untar(tarball); err == nil {
			t.Errorf("%v: untar() = %q, want error", name, dst)
		}
	}

	// the dot-rooted tarball must never resolve to the directory holding it
	if _, err := os.Stat(filepath.Join(dir, "init")); err == nil {
		t.Errorf("dot-rooted tarball was extracted into %v", dir)
	}
}

// TestCLIPreprocessTar covers the tar: prefix with an absolute path, which
// parses as a URL path rather than an opaque string.
func TestCLIPreprocessTar(t *testing.T) {
	dir := t.TempDir()
	tarball := writeTarball(t, dir, "fs.tgz", "rootfs/", "rootfs/init")

	got, err := cliPreprocess("tar:" + tarball)
	if err != nil {
		t.Fatalf("cliPreprocess: %v", err)
	}

	want := filepath.Join(dir, "rootfs")
	if got != want {
		t.Errorf("cliPreprocess() = %q, want %q", got, want)
	}

	if src := tarSource(want); src != tarball {
		t.Errorf("tarSource(%q) = %q, want %q", want, src, tarball)
	}
}

// TestGetFilesUntarsFilesystem checks that each host launching queued
// containers extracts the filesystem tarball itself.
func TestGetFilesUntarsFilesystem(t *testing.T) {
	dir := t.TempDir()
	tarball := writeTarball(t, dir, "fs.tar.gz", "rootfs/", "rootfs/init")

	q := &QueuedVMs{VMType: CONTAINER, FilesystemTar: tarball}
	q.FilesystemPath = "/nonexistent/rootfs"

	if err := q.GetFiles(""); err != nil {
		t.Fatalf("GetFiles: %v", err)
	}

	want := filepath.Join(dir, "rootfs")
	if q.FilesystemPath != want {
		t.Errorf("FilesystemPath = %q, want %q", q.FilesystemPath, want)
	}

	if _, err := os.Stat(filepath.Join(want, "init")); err != nil {
		t.Errorf("filesystem not extracted: %v", err)
	}
}

// TestQueueRecordsFilesystemTar checks that queued containers carry the
// tarball behind a tar: filesystem so that remote hosts can extract it.
func TestQueueRecordsFilesystemTar(t *testing.T) {
	dir := t.TempDir()
	tarball := writeTarball(t, dir, "fs.tar.gz", "rootfs/", "rootfs/init")

	fs, err := cliPreprocess("tar:" + tarball)
	if err != nil {
		t.Fatalf("cliPreprocess: %v", err)
	}

	n := &Namespace{Name: "tar-queue", Hosts: map[string]bool{}}

	config := NewVMConfig()
	config.FilesystemPath = fs

	if err := n.Queue("c1", CONTAINER, config); err != nil {
		t.Fatalf("Queue container: %v", err)
	}
	if err := n.Queue("k1", KVM, config); err != nil {
		t.Fatalf("Queue kvm: %v", err)
	}

	config.FilesystemPath = filepath.Join(dir, "elsewhere")
	if err := n.Queue("c2", CONTAINER, config); err != nil {
		t.Fatalf("Queue container: %v", err)
	}

	want := []string{tarball, "", ""}
	for i, q := range n.queue {
		if q.FilesystemTar != want[i] {
			t.Errorf("queue[%v] %v: FilesystemTar = %q, want %q", i, q.Names, q.FilesystemTar, want[i])
		}
	}
}

// TestUntarAbsoluteNames checks that tar's warning about absolute member
// names, printed on stderr, is not mistaken for a second top-level entry.
func TestUntarAbsoluteNames(t *testing.T) {
	dir := t.TempDir()
	tarball := writeTarball(t, dir, "abs.tar.gz", "/rootfs/", "/rootfs/init")

	dst, _, err := untar(tarball)
	if err != nil {
		t.Fatalf("untar: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "init")); err != nil || dst != filepath.Join(dir, "rootfs") {
		t.Errorf("untar() = %q, init: %v", dst, err)
	}
}

// TestGetFilesKeepsExtractedFilesystem checks that a host that already has the
// extracted filesystem launches without the tarball.
func TestGetFilesKeepsExtractedFilesystem(t *testing.T) {
	dir := t.TempDir()
	tarball := writeTarball(t, dir, "fs.tar.gz", "rootfs/", "rootfs/init")

	fs, err := cliPreprocess("tar:" + tarball)
	if err != nil {
		t.Fatalf("cliPreprocess: %v", err)
	}

	if err := os.Remove(tarball); err != nil {
		t.Fatal(err)
	}

	q := &QueuedVMs{VMType: CONTAINER, FilesystemTar: tarSource(fs)}
	q.FilesystemPath = fs

	if err := q.GetFiles(""); err != nil {
		t.Errorf("GetFiles: %v", err)
	}
	if q.FilesystemPath != fs {
		t.Errorf("FilesystemPath = %q, want %q", q.FilesystemPath, fs)
	}
}

// TestTarSourceKeepsExtractedTarball checks that a second tarball with the same
// top-level directory, which reuses the first one's files, does not replace
// the tarball that other hosts extract.
func TestTarSourceKeepsExtractedTarball(t *testing.T) {
	dir := t.TempDir()
	a := writeTarball(t, dir, "a.tar.gz", "rootfs/", "rootfs/a")
	b := writeTarball(t, dir, "b.tar.gz", "rootfs/", "rootfs/b")

	fa, err := cliPreprocess("tar:" + a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := cliPreprocess("tar:" + b)
	if err != nil {
		t.Fatal(err)
	}

	if fa != fb {
		t.Fatalf("filesystems %q and %q differ", fa, fb)
	}
	if src := tarSource(fa); src != a {
		t.Errorf("tarSource(%q) = %q, want %q", fa, src, a)
	}
}

// TestQueuedVMsGobFilesystemTar checks that FilesystemTar survives the gob
// encoding that sends queued VMs to other hosts.
func TestQueuedVMsGobFilesystemTar(t *testing.T) {
	var buf bytes.Buffer

	in := meshageVMLaunch{QueuedVMs: &QueuedVMs{VMType: CONTAINER, FilesystemTar: "live/fs.tar.gz"}}
	if err := gob.NewEncoder(&buf).Encode(in); err != nil {
		t.Fatal(err)
	}

	var out meshageVMLaunch
	if err := gob.NewDecoder(&buf).Decode(&out); err != nil {
		t.Fatal(err)
	}

	if out.QueuedVMs == nil || out.FilesystemTar != "live/fs.tar.gz" {
		t.Errorf("decoded %+v", out.QueuedVMs)
	}
}
