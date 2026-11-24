// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestInitrdWithFile checks that the new initrd is the old one plus a gzip
// cpio archive with the file, and that cpio can unpack that archive.
func TestInitrdWithFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "live.initrd")
	dst := filepath.Join(dir, "router.initrd")

	// 5 bytes, so that the helper must pad before the new archive
	base := []byte("12345")
	if err := os.WriteFile(src, base, 0644); err != nil {
		t.Fatal(err)
	}

	if err := InitrdWithFile(src, dst, "etc/live.role", []byte("router\n")); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, base) {
		t.Fatalf("the new initrd does not start with the old one")
	}

	rest := bytes.TrimLeft(got[len(base):], "\x00")
	if (len(got)-len(rest))%4 != 0 {
		t.Errorf("the new archive starts at offset %d, want a multiple of 4", len(got)-len(rest))
	}

	zr, err := gzip.NewReader(bytes.NewReader(rest))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}

	want := "070701" +
		"00000000" + "000081A4" + "00000000" + "00000000" + "00000001" + "00000000" +
		"00000007" + "00000000" + "00000000" + "00000000" + "00000000" + "0000000E" + "00000000" +
		"etc/live.role\x00" +
		"router\n" + "\x00"
	if !bytes.HasPrefix(archive, []byte(want)) {
		t.Errorf("archive = %q, want prefix %q", archive, want)
	}
	if !bytes.Contains(archive, []byte("TRAILER!!!\x00")) || len(archive)%4 != 0 {
		t.Errorf("archive has no trailer or a wrong length (%d)", len(archive))
	}

	// cpio is the reference reader, where it is installed
	cpio, err := exec.LookPath("cpio")
	if err != nil {
		t.Skip("cpio not found")
	}

	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(filepath.Join(out, "etc"), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(cpio, "-i", "--quiet")
	cmd.Dir = out
	cmd.Stdin = bytes.NewReader(archive)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cpio: %v: %s", err, b)
	}

	data, err := os.ReadFile(filepath.Join(out, "etc", "live.role"))
	if err != nil || string(data) != "router\n" {
		t.Errorf("unpacked file = %q, %v", data, err)
	}
}
