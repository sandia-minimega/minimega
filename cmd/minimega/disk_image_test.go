// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// diskImageTestEnv enables tests that mount real images via nbd (needs root).
const diskImageTestEnv = "MINIMEGA_TEST_DISK_IMAGES"

func requireDiskImageTests(t *testing.T) {
	t.Helper()

	if os.Getenv(diskImageTestEnv) == "" {
		t.Skipf("set %s=1 to run tests that mount disk images (needs root and nbd)", diskImageTestEnv)
	}

	if os.Geteuid() != 0 {
		t.Fatal("disk image tests must run as root")
	}

	for _, tool := range []string{"qemu-img", "qemu-nbd", "sfdisk", "mkfs.ext4", "mount", "blockdev"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("disk image tests need %s: %v", tool, err)
		}
	}
}

func run(t *testing.T, args ...string) {
	t.Helper()

	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %v: %s", args, err, out)
	}
}

func sha256File(t *testing.T, path string) string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// makeGuestImage builds a qcow2 image with one ext4 partition from makeGuestTree.
func makeGuestImage(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	tree := filepath.Join(dir, "tree")
	makeGuestTree(t, tree)

	// partition 1 starts at 1 MiB
	raw := filepath.Join(dir, "guest.raw")
	run(t, "truncate", "-s", "64M", raw)
	run(t, "sh", "-c", "echo 'start=2048, type=83' | sfdisk "+raw)
	run(t, "mkfs.ext4", "-q", "-F", "-E", "offset=1048576", "-d", tree, raw, "63M")

	img := filepath.Join(dir, "guest.qcow2")
	run(t, "qemu-img", "convert", "-f", "raw", "-O", "qcow2", raw, img)

	return img
}

func mountedUnder(t *testing.T, dir string) []string {
	t.Helper()

	b, err := os.ReadFile("/proc/mounts")
	if err != nil {
		t.Fatal(err)
	}

	var res []string
	for _, line := range strings.Split(string(b), "\n") {
		if fields := strings.Fields(line); len(fields) > 1 && strings.HasPrefix(fields[1], dir) {
			res = append(res, fields[1])
		}
	}

	return res
}

func TestDiskImageExtractInject(t *testing.T) {
	requireDiskImageTests(t)

	originalBase := *f_base
	*f_base = t.TempDir()
	defer func() { *f_base = originalBase }()

	img := makeGuestImage(t)
	before := sha256File(t, img)

	out := t.TempDir()

	t.Run("extract", func(t *testing.T) {
		pairs := map[string]string{
			filepath.Join(out, "hostname"): "/etc/hostname",
			filepath.Join(out, "data"):     "/data",
			filepath.Join(out, "abs-link"): "/abs-link",
			filepath.Join(out, "climb"):    "/var/up/etc/hostname",
		}

		if err := diskExtract(img, "", pairs, nil); err != nil {
			t.Fatalf("diskExtract: %v", err)
		}

		if got := readFile(t, filepath.Join(out, "hostname")); got != "guest\n" {
			t.Errorf("hostname = %q, want %q", got, "guest\n")
		}
		if got := readFile(t, filepath.Join(out, "data/sub/b.txt")); got != "b\n" {
			t.Errorf("data/sub/b.txt = %q, want %q", got, "b\n")
		}
		if got := readFile(t, filepath.Join(out, "climb")); got != "guest\n" {
			t.Errorf("climb = %q, want the guest's /etc/hostname", got)
		}
		if target, err := os.Readlink(filepath.Join(out, "abs-link")); err != nil || target != "/etc/hostname" {
			t.Errorf("abs-link = %q, %v; want a symlink to /etc/hostname", target, err)
		}
	})

	t.Run("extract errors", func(t *testing.T) {
		cases := []struct {
			name      string
			partition string
			source    string
			want      string
		}{
			{name: "fifo", source: "/special/fifo", want: "special file"},
			{name: "missing source", source: "/nope", want: "error accessing '/nope'"},
			{name: "missing partition", partition: "2", source: "/etc/hostname", want: "desired partition 2 not found"},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				pairs := map[string]string{filepath.Join(t.TempDir(), "x"): c.source}

				err := diskExtract(img, c.partition, pairs, nil)
				if err == nil || !strings.Contains(err.Error(), c.want) {
					t.Fatalf("diskExtract error = %v, want it to contain %q", err, c.want)
				}
			})
		}
	})

	t.Run("extract with -w", func(t *testing.T) {
		pairs := map[string]string{filepath.Join(t.TempDir(), "x"): "/etc/hostname"}

		if err := diskExtract(img, "1", pairs, []string{"-w"}); err != nil {
			t.Fatalf("diskExtract: %v", err)
		}
	})

	if after := sha256File(t, img); after != before {
		t.Errorf("disk extract modified the image: sha256 %s, want %s", after, before)
	}

	t.Run("inject then extract", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "injected.txt")
		if err := os.WriteFile(src, []byte("injected\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := diskInject(img, "", map[string]string{"/opt/injected.txt": src}, nil, false, nil); err != nil {
			t.Fatalf("diskInject: %v", err)
		}

		dst := filepath.Join(t.TempDir(), "injected.txt")
		if err := diskExtract(img, "", map[string]string{dst: "/opt/injected.txt"}, nil); err != nil {
			t.Fatalf("diskExtract: %v", err)
		}

		if got := readFile(t, dst); got != "injected\n" {
			t.Errorf("injected.txt = %q, want %q", got, "injected\n")
		}

		if err := diskInject(img, "", nil, nil, true, []string{"/opt/injected.txt"}); err != nil {
			t.Fatalf("diskInject delete: %v", err)
		}

		err := diskExtract(img, "", map[string]string{dst + ".2": "/opt/injected.txt"}, nil)
		if err == nil || !strings.Contains(err.Error(), "error accessing") {
			t.Errorf("deleted file still extracted: %v", err)
		}
	})

	if mounts := mountedUnder(t, *f_base); len(mounts) != 0 {
		t.Errorf("mounts left behind: %v", mounts)
	}

	if entries, err := os.ReadDir(*f_base); err != nil || len(entries) != 0 {
		t.Errorf("mount points left behind in %s: %v, %v", *f_base, entries, err)
	}
}
