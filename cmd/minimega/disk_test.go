// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackingName(t *testing.T) {
	cases := []struct {
		image, backing string
		abs            bool
		want           string
	}{
		{"/b/files/saved/a.hdd", "/b/files/x.qcow2", false, "../x.qcow2"},
		{"/b/files/saved/a.hdd", "/b/files/saved/x.qcow2", false, "x.qcow2"},
		{"/b/1/disk-0.qcow2", "/b/files/live/x.qcow2", false, "../files/live/x.qcow2"},
		{"/b/files/saved/a.hdd", "/b/files/x.qcow2", true, "/b/files/x.qcow2"},
		{"/b/files/a.hdd", "/b/files/ubuntu:22.04.qcow2", false, "./ubuntu:22.04.qcow2"},
	}

	for _, c := range cases {
		got, err := backingName(c.image, c.backing, c.abs)
		if err != nil {
			t.Errorf("backingName(%q, %q, %v): %v", c.image, c.backing, c.abs, err)
			continue
		}
		if got != c.want {
			t.Errorf("backingName(%q, %q, %v) = %q, want %q", c.image, c.backing, c.abs, got, c.want)
		}
	}
}

// TestSavedDiskBacking uses a directory tree in which "/b" is a temporary
// directory. The files need not be images.
func TestSavedDiskBacking(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"13", "files/saved", "files/a:b", "other/deep", "store/live", "elsewhere/saved", "elsewhere/deep"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{
		"link":           "other/deep",
		"files/live":     "../store/live",
		"files/slink":    "../elsewhere/saved",
		"files/dlink":    "../elsewhere/deep",
		"files/saved/up": "..",
	} {
		if err := os.Symlink(target, filepath.Join(base, link)); err != nil {
			t.Fatal(err)
		}
	}
	b := func(s string) string { return strings.ReplaceAll(s, "/b/", base+"/") }

	const image = "/b/files/saved/a.hdd"

	cases := []struct {
		name        string
		image       string
		stored      string
		abs         bool
		wantImage   string
		wantBacking string
		wantRebase  bool
	}{
		{"no backing file", image, "", false, "", "", false},
		{"no backing file, absolute", image, "", true, "", "", false},
		{"absolute, clean", image, "/b/files/x.qcow2", true, image, "/b/files/x.qcow2", false},
		{"absolute, through instance dir", image, "/b/13/../files/x.qcow2", true, image, "/b/files/x.qcow2", true},
		{"absolute, stored relative", image, "../x.qcow2", true, image, "/b/files/x.qcow2", true},
		{"relative, through instance dir", image, "/b/13/../files/x.qcow2", false, image, "/b/files/x.qcow2", true},
		{"relative, clean", image, "../x.qcow2", false, image, "/b/files/x.qcow2", false},
		{"relative, unclean", image, "../saved/../x.qcow2", false, image, "/b/files/x.qcow2", true},
		{"relative, stored absolute", image, "/b/files/x.qcow2", false, image, "/b/files/x.qcow2", true},
		{"protocol name", image, "nbd://host/x", false, "", "", false},
		{"protocol name, absolute", image, "nbd://host/x", true, "", "", false},
		{"protocol name, no slashes", image, "nbd:localhost:10809", false, "", "", false},
		{"protocol name, socket", image, "nbd:unix:/b/13/../sock", true, "", "", false},
		{"relative path with a colon in a directory", image, "../a:b/x.qcow2", false, image, "/b/files/a:b/x.qcow2", false},
		{"relative, colon in a name in the same directory", image, "/b/13/../files/saved/ubuntu:22.04.qcow2", false, image, "/b/files/saved/ubuntu:22.04.qcow2", true},
		{"relative, clean colon in a name in the same directory", image, "./ubuntu:22.04.qcow2", false, image, "/b/files/saved/ubuntu:22.04.qcow2", false},
		{"json name", image, `json:{"driver":"qcow2","file":{"filename":"/b/13/../files/x.qcow2"}}`, false, "", "", false},

		// the symlink, not the clean path, decides the parent of a ".."
		{"absolute, symlink before ..", image, "/b/link/../x.qcow2", true, image, "/b/other/x.qcow2", true},
		{"relative, symlink before ..", image, "/b/link/../x.qcow2", false, image, "/b/other/x.qcow2", true},
		{"relative, stored relative, symlink before ..", image, "up/../x.qcow2", false, image, "/b/x.qcow2", true},
		{"absolute, clean path through a symlink", image, "/b/files/live/x.qcow2", true, image, "/b/files/live/x.qcow2", false},
		{"relative, clean path through a symlink", image, "../live/x.qcow2", false, image, "/b/files/live/x.qcow2", false},
		{"relative, saved in a symlinked directory", "/b/files/slink/a.hdd", "/b/13/../files/x.qcow2", false, "/b/elsewhere/saved/a.hdd", "/b/files/x.qcow2", true},
		{"absolute, saved in a symlinked directory", "/b/files/slink/a.hdd", "/b/13/../files/x.qcow2", true, "/b/files/slink/a.hdd", "/b/files/x.qcow2", true},
		{"relative, saved through a symlink before ..", "/b/files/dlink/../saved/a.hdd", "../x.qcow2", false, "/b/elsewhere/saved/a.hdd", "/b/elsewhere/x.qcow2", false},
		{"absolute, saved through a symlink before ..", "/b/files/dlink/../saved/a.hdd", "/b/13/../files/x.qcow2", true, "/b/elsewhere/saved/a.hdd", "/b/files/x.qcow2", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			img, backing, rebase, err := savedDiskBacking(b(c.image), b(c.stored), c.abs)
			if err != nil {
				t.Fatal(err)
			}
			if img != b(c.wantImage) || backing != b(c.wantBacking) || rebase != c.wantRebase {
				t.Errorf("savedDiskBacking(%q, %q, %v) = %q, %q, %v, want %q, %q, %v",
					c.image, c.stored, c.abs, img, backing, rebase, b(c.wantImage), b(c.wantBacking), c.wantRebase)
			}
		})
	}
}

// TestFixSavedDiskBacking makes the image chain that "vm save" makes with
// qemu-img, then removes the VM instance directory as "vm flush" does.
func TestFixSavedDiskBacking(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not found")
	}

	for _, abs := range []bool{false, true} {
		name := "relative"
		if abs {
			name = "absolute"
		}

		t.Run(name, func(t *testing.T) {
			setDiskFlags(t, abs)

			base := t.TempDir()
			files := filepath.Join(base, "files")
			*f_iomBase = files
			data := filepath.Join(files, "live", "data.qcow2")
			instance := filepath.Join(base, "13")
			overlay := filepath.Join(instance, "disk-0.qcow2")
			saved := filepath.Join(files, "saved", "c2.hdd")
			for _, d := range []string{filepath.Dir(data), instance, filepath.Dir(saved)} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			qemuImg(t, "create", "-f", "qcow2", data, "1M")
			qemuImg(t, "create", "-f", "qcow2", "-b", "../files/live/data.qcow2", "-F", "qcow2", overlay)
			// the backing file name that QEMU writes in the drive-backup target
			unclean := instance + "/../files/live/data.qcow2"
			qemuImg(t, "create", "-f", "qcow2", "-b", unclean, "-F", "qcow2", saved)
			if info, err := diskInfo(saved); err != nil || info.BackingFile != unclean {
				t.Fatalf("before fix: backing file = %q, %v, want %q", info.BackingFile, err, unclean)
			}

			if err := fixSavedDiskBacking(saved); err != nil {
				t.Fatal(err)
			}

			if err := os.RemoveAll(instance); err != nil {
				t.Fatal(err)
			}

			if out, err := exec.Command("qemu-img", "info", "--backing-chain", saved).CombinedOutput(); err != nil {
				t.Fatalf("saved disk is not usable without the instance directory: %v\n%s", err, out)
			}

			want := "../live/data.qcow2"
			if abs {
				want = data
			}
			info, err := diskInfo(saved)
			if err != nil {
				t.Fatal(err)
			}
			if info.BackingFile != want {
				t.Errorf("backing file = %q, want %q", info.BackingFile, want)
			}
		})
	}

	t.Run("relative, colon in the backing name", func(t *testing.T) {
		setDiskFlags(t, false)

		base := t.TempDir()
		saved := filepath.Join(base, "files", "saved", "c2.hdd")
		*f_iomBase = filepath.Join(base, "files")
		data := filepath.Join(filepath.Dir(saved), "ubuntu:22.04.qcow2")
		instance := filepath.Join(base, "13")
		for _, d := range []string{filepath.Dir(saved), instance} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}

		qemuImg(t, "create", "-f", "qcow2", data, "1M")
		qemuImg(t, "create", "-f", "qcow2", "-b", instance+"/../files/saved/ubuntu:22.04.qcow2", "-F", "qcow2", saved)

		if err := fixSavedDiskBacking(saved); err != nil {
			t.Fatal(err)
		}

		if err := os.RemoveAll(instance); err != nil {
			t.Fatal(err)
		}

		if out, err := exec.Command("qemu-img", "info", "--backing-chain", saved).CombinedOutput(); err != nil {
			t.Fatalf("saved disk is not usable: %v\n%s", err, out)
		}

		info, err := diskInfo(saved)
		if err != nil {
			t.Fatal(err)
		}
		if want := "./ubuntu:22.04.qcow2"; info.BackingFile != want {
			t.Errorf("backing file = %q, want %q", info.BackingFile, want)
		}
	})
}

// TestFixSavedDiskBackingRelativePath gives fixSavedDiskBacking a path that is
// relative to the current directory.
func TestFixSavedDiskBackingRelativePath(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not found")
	}

	setDiskFlags(t, false)

	base := t.TempDir()
	*f_iomBase = filepath.Join(base, "files")
	data := filepath.Join(base, "files", "live", "data.qcow2")
	instance := filepath.Join(base, "13")
	saved := filepath.Join("files", "saved", "c2.hdd")
	for _, d := range []string{filepath.Dir(data), instance, filepath.Join(base, filepath.Dir(saved))} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	qemuImg(t, "create", "-f", "qcow2", data, "1M")
	qemuImg(t, "create", "-f", "qcow2", "-b", instance+"/../files/live/data.qcow2", "-F", "qcow2", filepath.Join(base, saved))

	t.Chdir(base)

	if err := fixSavedDiskBacking(saved); err != nil {
		t.Fatal(err)
	}

	info, err := diskInfo(saved)
	if err != nil {
		t.Fatal(err)
	}
	if want := "../live/data.qcow2"; info.BackingFile != want {
		t.Errorf("backing file = %q, want %q", info.BackingFile, want)
	}
}

// TestFixSavedDiskBackingSymlinks puts a symlink where a clean path names
// another image than QEMU opens. Each wrong image is 2M and the backing file
// of the saved disk is 1M.
func TestFixSavedDiskBackingSymlinks(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not found")
	}

	cases := []struct {
		name  string
		abs   bool
		dirs  []string
		links map[string]string
		// paths relative to the base; stored starts at the instance directory
		data, wrong, stored, saved string
	}{
		{
			name:   "symlink before .. in the backing name",
			dirs:   []string{"other/deep", "files/saved"},
			links:  map[string]string{"files/link": "../other/deep"},
			data:   "other/data.qcow2",
			wrong:  "files/data.qcow2",
			stored: "$i/../files/link/../data.qcow2",
			saved:  "files/saved/c2.hdd",
		},
		{
			name:   "symlinked save directory",
			dirs:   []string{"files/live", "elsewhere/saved", "elsewhere/live"},
			links:  map[string]string{"files/saved": "../elsewhere/saved"},
			data:   "files/live/data.qcow2",
			wrong:  "elsewhere/live/data.qcow2",
			stored: "$i/../files/live/data.qcow2",
			saved:  "files/saved/c2.hdd",
		},
		{
			name:   "symlink before .. in the saved disk path",
			dirs:   []string{"files/live", "files/saved", "elsewhere/deep", "elsewhere/saved"},
			links:  map[string]string{"files/dlink": "../elsewhere/deep"},
			data:   "files/live/data.qcow2",
			wrong:  "files/saved/c2.hdd",
			stored: "$i/../files/live/data.qcow2",
			saved:  "files/dlink/../saved/c2.hdd",
		},
	}

	for _, c := range cases {
		for _, abs := range []bool{false, true} {
			name := c.name + ", relative"
			if abs {
				name = c.name + ", absolute"
			}

			t.Run(name, func(t *testing.T) {
				setDiskFlags(t, abs)

				base := t.TempDir()
				*f_iomBase = filepath.Join(base, "files")
				instance := filepath.Join(base, "13")
				for _, d := range append([]string{"13"}, c.dirs...) {
					if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				for link, target := range c.links {
					if err := os.Symlink(target, filepath.Join(base, link)); err != nil {
						t.Fatal(err)
					}
				}

				qemuImg(t, "create", "-f", "qcow2", filepath.Join(base, c.data), "1M")
				qemuImg(t, "create", "-f", "qcow2", filepath.Join(base, c.wrong), "2M")
				saved := base + "/" + c.saved
				stored := strings.ReplaceAll(c.stored, "$i", instance)
				qemuImg(t, "create", "-f", "qcow2", "-b", stored, "-F", "qcow2", saved)

				if err := fixSavedDiskBacking(saved); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(instance); err != nil {
					t.Fatal(err)
				}

				if got := backingSize(t, saved); got != 1<<20 {
					t.Errorf("backing file of the saved disk has size %d, want %d", got, 1<<20)
				}
			})
		}
	}
}

// backingSize returns the virtual size of the backing file that QEMU opens for
// image.
func backingSize(t *testing.T, image string) int64 {
	t.Helper()

	out, err := exec.Command("qemu-img", "info", "--backing-chain", "--output=json", image).Output()
	if err != nil {
		t.Fatalf("saved disk is not usable: %v", err)
	}

	var chain []struct {
		Size int64 `json:"virtual-size"`
	}
	if err := json.Unmarshal(out, &chain); err != nil {
		t.Fatal(err)
	}
	if len(chain) < 2 {
		t.Fatalf("saved disk has no backing file: %s", out)
	}

	return chain[1].Size
}

func setDiskFlags(t *testing.T, abs bool) {
	t.Helper()

	oldAbs, oldBase := *f_absSnapshot, *f_iomBase
	t.Cleanup(func() {
		*f_absSnapshot, *f_iomBase = oldAbs, oldBase
	})

	*f_absSnapshot = abs
}

func qemuImg(t *testing.T, args ...string) {
	t.Helper()

	if out, err := exec.Command("qemu-img", args...).CombinedOutput(); err != nil {
		t.Fatalf("qemu-img %v: %v\n%s", args, err, out)
	}
}
