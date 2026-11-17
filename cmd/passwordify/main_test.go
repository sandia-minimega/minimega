// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteSSHDConfig(t *testing.T) {
	tests := []struct {
		name, in, want string
		changed        bool
	}{
		{"enabled", "PermitRootLogin yes\nPermitEmptyPasswords yes\nUsePAM yes\n", "PermitRootLogin yes\nPermitEmptyPasswords no\nUsePAM yes\n", true},
		{"case insensitive", "permitemptypasswords YES\n", "PermitEmptyPasswords no\n", true},
		{"equals and tab", "PermitEmptyPasswords=yes\nX\tyes\n", "PermitEmptyPasswords no\nX\tyes\n", true},
		{"indented in match block", "Match User root\n\tPermitEmptyPasswords yes\n", "Match User root\n\tPermitEmptyPasswords no\n", true},
		{"quoted", "\"PermitEmptyPasswords\" \"yes\"\nPermitEmptyPasswords=\"YES\"\n", "PermitEmptyPasswords no\nPermitEmptyPasswords no\n", true},
		{"quoted no", "PermitEmptyPasswords \"no\"\n", "PermitEmptyPasswords \"no\"\n", false},
		{"crlf and no final newline", "A b\r\nPermitEmptyPasswords yes\r\nPermitEmptyPasswords  yes", "A b\r\nPermitEmptyPasswords no\r\nPermitEmptyPasswords no", true},
		{"already disabled", "PermitEmptyPasswords no\n", "PermitEmptyPasswords no\n", false},
		{"commented out", "#PermitEmptyPasswords yes\n", "#PermitEmptyPasswords yes\n", false},
		{"other option", "PasswordAuthentication yes\n", "PasswordAuthentication yes\n", false},
		{"empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := rewriteSSHDConfig(tt.in)
			if got != tt.want || changed != tt.changed {
				t.Errorf("rewriteSSHDConfig(%q) = %q, %v; want %q, %v", tt.in, got, changed, tt.want, tt.changed)
			}
		})
	}
}

func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root
}

// TestImageSymlinksStayInImage checks that absolute symlinks in the image are
// not followed to the host, where passwordify runs as root.
func TestImageSymlinksStayInImage(t *testing.T) {
	host := t.TempDir()
	hostConfig := filepath.Join(host, "sshd_config")
	if err := os.WriteFile(hostConfig, []byte("PermitEmptyPasswords yes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	keys := filepath.Join(t.TempDir(), "keys")
	if err := os.WriteFile(keys, []byte("ssh-ed25519 AAAA key\n"), 0644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "etc/ssh"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(hostConfig, filepath.Join(dir, "etc/ssh/sshd_config")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "root"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(host, filepath.Join(dir, "root/.ssh")); err != nil {
		t.Fatal(err)
	}

	if err := disableEmptyPasswords(openRoot(t, dir)); err == nil {
		t.Error("disableEmptyPasswords followed a symlink out of the image")
	}
	if err := installKeys(openRoot(t, dir), keys); err == nil {
		t.Error("installKeys followed a symlink out of the image")
	}
	if _, err := exec.LookPath("ssh-keygen"); err == nil {
		if err := addPasswordless(openRoot(t, dir)); err == nil {
			t.Error("addPasswordless followed a symlink out of the image")
		}
	}

	if b, err := os.ReadFile(hostConfig); err != nil || string(b) != "PermitEmptyPasswords yes\n" {
		t.Errorf("host sshd_config changed: %q, %v", b, err)
	}
	if entries, err := os.ReadDir(host); err != nil || len(entries) != 1 {
		t.Errorf("files written to the host directory: %v, %v", entries, err)
	}
}

func TestDisableEmptyPasswords(t *testing.T) {
	dir := t.TempDir()

	// no sshd in the image is not an error
	if err := disableEmptyPasswords(openRoot(t, dir)); err != nil {
		t.Fatalf("without sshd_config: %v", err)
	}

	path := filepath.Join(dir, "etc/ssh/sshd_config")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("UsePAM yes\nPermitEmptyPasswords yes\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := disableEmptyPasswords(openRoot(t, dir)); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "UsePAM yes\nPermitEmptyPasswords no\n"; string(b) != want {
		t.Errorf("sshd_config = %q, want %q", b, want)
	}
	if fi, err := os.Stat(path); err != nil {
		t.Error(err)
	} else if fi.Mode().Perm() != 0600 {
		t.Errorf("sshd_config mode changed to %v", fi.Mode().Perm())
	}
}

func TestReadPasswordLine(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"secret\n", "secret", true},
		{"secret\r\n", "secret", true},
		{"no newline", "no newline", true},
		{"first\nsecond\n", "first", true},
		{"spaces & $HOME symbols\n", "spaces & $HOME symbols", true},
		{"", "", false},
		{"\n", "", false},
		{"\r\n", "", false},
	}

	for _, tt := range tests {
		got, err := readPasswordLine(strings.NewReader(tt.in))
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("readPasswordLine(%q) = %q, %v; want %q, ok=%v", tt.in, got, err, tt.want, tt.ok)
		}
	}
}

func TestCheckPassword(t *testing.T) {
	for _, pw := range []string{"", "a\nb", "a\rb"} {
		if checkPassword(pw) == nil {
			t.Errorf("checkPassword(%q) accepted an invalid password", pw)
		}
	}
	if err := checkPassword("ok"); err != nil {
		t.Errorf("checkPassword(ok) = %v", err)
	}
}

func TestInstallKeys(t *testing.T) {
	dir := t.TempDir()
	keys := filepath.Join(t.TempDir(), "keys")
	if err := os.WriteFile(keys, []byte("ssh-ed25519 AAAA short\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// creates root/.ssh when the image has none
	if err := installKeys(openRoot(t, dir), keys); err != nil {
		t.Fatal(err)
	}
	sshDir := filepath.Join(dir, "root/.ssh")
	if fi, err := os.Stat(sshDir); err != nil {
		t.Fatal(err)
	} else if fi.Mode().Perm() != 0700 {
		t.Errorf("root/.ssh has mode %v, want 0700", fi.Mode().Perm())
	}

	// replaces a longer existing file instead of leaving its tail behind
	ak := filepath.Join(sshDir, "authorized_keys")
	if err := os.WriteFile(ak, []byte(strings.Repeat("old key line\n", 10)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := installKeys(openRoot(t, dir), keys); err != nil {
		t.Fatalf("with an existing root/.ssh: %v", err)
	}
	b, err := os.ReadFile(ak)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "ssh-ed25519 AAAA short\n" {
		t.Errorf("authorized_keys = %q", b)
	}

	if err := installKeys(openRoot(t, dir), filepath.Join(dir, "missing")); err == nil {
		t.Error("installKeys with a missing key file did not fail")
	}
}

func TestAddPasswordless(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not found")
	}

	dir := t.TempDir()
	sshDir := filepath.Join(dir, "root/.ssh")
	ak := filepath.Join(sshDir, "authorized_keys")

	// works without an existing root/.ssh or authorized_keys
	if err := addPasswordless(openRoot(t, dir)); err != nil {
		t.Fatal(err)
	}

	// a replaced key gets the private key mode even if the image's was too open
	if err := os.Chmod(filepath.Join(sshDir, "id_rsa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ak, 0666); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for path, want := range map[string]os.FileMode{"id_rsa": 0600, "id_rsa.pub": 0644, "authorized_keys": 0600} {
			if fi, err := os.Stat(filepath.Join(sshDir, path)); err != nil {
				t.Error(err)
			} else if fi.Mode().Perm() != want {
				t.Errorf("%v has mode %v, want %v", path, fi.Mode().Perm(), want)
			}
		}
	}()
	first, err := os.ReadFile(filepath.Join(sshDir, "id_rsa.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(first), "ssh-rsa ") {
		t.Errorf("id_rsa.pub is not an RSA key: %q", first)
	}

	// a second run replaces the key without prompting and keeps earlier keys
	if err := addPasswordless(openRoot(t, dir)); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(sshDir, "id_rsa.pub"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(ak)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(first)+string(second) {
		t.Errorf("authorized_keys = %q, want both generated keys", b)
	}
}

func TestUnpackRepack(t *testing.T) {
	for _, tool := range []string{"bash", "cpio", "gzip", "zcat", "find"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%v not found", tool)
		}
	}

	src := t.TempDir()
	if err := os.Chmod(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "etc/ssh"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "etc/ssh/sshd_config"), []byte("UsePAM yes\n"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("etc/ssh", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}

	// the path is passed to the shell as an argument, so spaces are fine
	initrd := filepath.Join(t.TempDir(), "my initrd.gz")
	if err := repack(src, initrd); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	if err := unpack(initrd, dst); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(dst, "etc/ssh/sshd_config"))
	if err != nil || string(b) != "UsePAM yes\n" {
		t.Errorf("sshd_config = %q, %v", b, err)
	}
	for path, want := range map[string]os.FileMode{".": 0755, "etc/ssh": 0750, "etc/ssh/sshd_config": 0640} {
		if fi, err := os.Stat(filepath.Join(dst, path)); err != nil {
			t.Error(err)
		} else if fi.Mode().Perm() != want {
			t.Errorf("%v has mode %v, want %v", path, fi.Mode().Perm(), want)
		}
	}
	if target, err := os.Readlink(filepath.Join(dst, "link")); err != nil || target != "etc/ssh" {
		t.Errorf("link = %q, %v", target, err)
	}

	if err := unpack(filepath.Join(dst, "missing"), t.TempDir()); err == nil {
		t.Error("unpack of a missing archive did not fail")
	}
	if err := repack(src, filepath.Join(dst, "missing", "out")); err == nil {
		t.Error("repack into a missing directory did not fail")
	}
}
