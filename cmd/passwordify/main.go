// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/term"
)

var (
	f_keys         = flag.String("keys", "", "authorized_keys formatted file to install for root")
	f_passwordless = flag.Bool("passwordless", true, "True if image should contain its own id_rsa.pub in authorized_keys for passwordless ssh between nodes")
)

func usage() {
	fmt.Println("usage: passwordify [option]... [source initramfs] [destination initramfs]")
	fmt.Println("\nThe root password is prompted for on a terminal, or read from the first line of stdin otherwise.")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	log.Init()

	if flag.NArg() != 2 {
		usage()
		os.Exit(1)
	}

	// chroot requires root
	if os.Geteuid() != 0 {
		log.Fatalln("passwordify must be run as root")
	}

	if err := externalCheck(*f_passwordless); err != nil {
		log.Fatalln(err)
	}

	// the unpack and repack commands run in the working directory
	source, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		log.Fatalln(err)
	}
	destination, err := filepath.Abs(flag.Arg(1))
	if err != nil {
		log.Fatalln(err)
	}

	// Make sure the source exists
	if _, err := os.Stat(source); err != nil {
		log.Fatalln("cannot find source initramfs", source, ":", err)
	}

	password, err := readPassword(os.Stdin)
	if err != nil {
		log.Fatalln(err)
	}

	if err := passwordify(source, destination, password, *f_keys, *f_passwordless); err != nil {
		log.Fatalln(err)
	}
}

// passwordify unpacks source, installs the password and keys, and repacks it
// to destination.
func passwordify(source, destination, password, keys string, passwordless bool) error {
	// Working directory
	tdir, err := os.MkdirTemp("", "passwordify")
	if err != nil {
		return fmt.Errorf("cannot create tempdir: %w", err)
	}
	defer os.RemoveAll(tdir)

	// MkdirTemp creates the directory 0700, which would become the image's / if the archive has no "." entry
	if err := os.Chmod(tdir, 0755); err != nil {
		return err
	}

	if err := unpack(source, tdir); err != nil {
		return err
	}

	if err := setPassword(tdir, password); err != nil {
		return err
	}

	// edits go through an os.Root so the image's symlinks cannot reach host files
	root, err := os.OpenRoot(tdir)
	if err != nil {
		return err
	}
	defer root.Close()

	if err := disableEmptyPasswords(root); err != nil {
		return err
	}

	// If keyfile, copy keyfile
	if keys != "" {
		if err := installKeys(root, keys); err != nil {
			return err
		}
	}

	// Generate an ssh key and append it to authorized_keys for
	// passwordless login between hosts running this initrd.
	if passwordless {
		if err := addPasswordless(root); err != nil {
			return err
		}
	}

	return repack(tdir, destination)
}

// readPassword prompts twice for the root password on a terminal, or reads
// the first line of input when it is not one.
func readPassword(in *os.File) (string, error) {
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		return readPasswordLine(in)
	}

	for {
		fmt.Printf("Enter new root password: ")
		pw1, err := term.ReadPassword(fd)
		if err != nil {
			return "", err
		}
		fmt.Printf("\nRetype new root password: ")
		pw2, err := term.ReadPassword(fd)
		if err != nil {
			return "", err
		}
		fmt.Printf("\n")
		if string(pw1) != string(pw2) {
			log.Errorln("passwords do not match")
		} else if err := checkPassword(string(pw1)); err != nil {
			log.Errorln(err)
		} else {
			return string(pw1), nil
		}
	}
}

// readPasswordLine reads a password from the first line of r.
func readPasswordLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", fmt.Errorf("unable to read password from stdin: %w", err)
	}
	pw := strings.TrimRight(line, "\r\n")
	if err := checkPassword(pw); err != nil {
		return "", err
	}
	return pw, nil
}

func checkPassword(pw string) error {
	if pw == "" {
		return errors.New("password must not be blank")
	}
	// passwd reads the password one line at a time
	if strings.ContainsAny(pw, "\r\n") {
		return errors.New("password must not contain a newline")
	}
	return nil
}

// unpack extracts the gzipped cpio archive source into dir.
func unpack(source, dir string) error {
	cmd := exec.Command("bash", "-c", "set -o pipefail; zcat -- \"$1\" | cpio -idm --quiet", "unpack", source)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("unable to unpack %v: %w: %s", source, err, output)
	}
	return nil
}

// repack archives dir as a gzipped newc cpio archive at destination.
func repack(dir, destination string) error {
	cmd := exec.Command("bash", "-c", "set -o pipefail; find . -print0 | cpio --quiet --null -o --format=newc | gzip -9 > \"$1\"", "repack", destination)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("unable to repack %v: %w: %s", destination, err, output)
	}
	return nil
}

// setPassword runs passwd in the image to set root's password.
func setPassword(dir, password string) error {
	cmd := exec.Command(process("chroot"), dir, "passwd")
	cmd.Stdin = strings.NewReader(password + "\n" + password + "\n")
	output, err := cmd.CombinedOutput()
	log.Info("passwd: %s", output)
	if err != nil {
		return fmt.Errorf("unable to set root password: %w: %s", err, output)
	}
	return nil
}

// disableEmptyPasswords turns off "PermitEmptyPasswords yes" in the image's
// sshd_config: with it, sshd accepts a password over keyboard-interactive but
// then fails the login, so clients never reach the password method.
func disableEmptyPasswords(root *os.Root) error {
	const path = "etc/ssh/sshd_config"
	fi, err := root.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("unable to check %v: %w", path, err)
	}

	b, err := readFile(root, path)
	if err != nil {
		return err
	}

	out, changed := rewriteSSHDConfig(string(b))
	if !changed {
		return nil
	}

	log.Info("setting PermitEmptyPasswords no in %v", path)
	return writeFile(root, path, []byte(out), os.O_TRUNC, fi.Mode().Perm())
}

// rewriteSSHDConfig replaces every enabled "PermitEmptyPasswords yes" with
// "PermitEmptyPasswords no" and reports whether it changed anything.
func rewriteSSHDConfig(config string) (string, bool) {
	lines := strings.SplitAfter(config, "\n")
	changed := false
	for i, line := range lines {
		fields := strings.FieldsFunc(line, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '=' || r == '\r' || r == '\n'
		})
		if len(fields) < 2 || !strings.EqualFold(unquote(fields[0]), "PermitEmptyPasswords") || !strings.EqualFold(unquote(fields[1]), "yes") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		eol := line[len(strings.TrimRight(line, "\r\n")):]
		lines[i] = indent + "PermitEmptyPasswords no" + eol
		changed = true
	}
	return strings.Join(lines, ""), changed
}

// unquote strips the double quotes sshd allows around a keyword or argument.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// installKeys replaces root's authorized_keys with the contents of keys.
func installKeys(root *os.Root, keys string) error {
	b, err := os.ReadFile(keys)
	if err != nil {
		return fmt.Errorf("can't open key file source: %w", err)
	}

	if err := mkdirSSH(root); err != nil {
		return err
	}

	if err := writeFile(root, "root/.ssh/authorized_keys", b, os.O_TRUNC, 0600); err != nil {
		return fmt.Errorf("couldn't write authorized_keys: %w", err)
	}
	return nil
}

// addPasswordless generates root's id_rsa for the image and authorizes it, so
// nodes booted from the image can ssh to each other.
func addPasswordless(root *os.Root) error {
	if err := mkdirSSH(root); err != nil {
		return err
	}

	// generate outside the image, since ssh-keygen would follow the image's symlinks
	kdir, err := os.MkdirTemp("", "passwordify-key")
	if err != nil {
		return err
	}
	defer os.RemoveAll(kdir)

	keyfile := filepath.Join(kdir, "id_rsa")
	cmd := exec.Command(process("ssh-keygen"), "-q", "-t", "rsa", "-f", keyfile, "-N", "")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ssh-keygen: %w: %s", err, output)
	}

	key, err := os.ReadFile(keyfile)
	if err != nil {
		return err
	}
	pub, err := os.ReadFile(keyfile + ".pub")
	if err != nil {
		return err
	}

	if err := writeFile(root, "root/.ssh/id_rsa", key, os.O_TRUNC, 0600); err != nil {
		return err
	}
	if err := writeFile(root, "root/.ssh/id_rsa.pub", pub, os.O_TRUNC, 0644); err != nil {
		return err
	}
	return writeFile(root, "root/.ssh/authorized_keys", pub, os.O_APPEND, 0600)
}

// mkdirSSH creates root/.ssh in the image if it does not exist.
func mkdirSSH(root *os.Root) error {
	for _, dir := range []struct {
		path string
		perm os.FileMode
	}{{"root", 0700}, {"root/.ssh", 0700}} {
		if err := root.Mkdir(dir.path, dir.perm); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("can't make root's ssh directory: %w", err)
		}
	}
	return nil
}

func readFile(root *os.Root, path string) ([]byte, error) {
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// writeFile writes b to path in root, creating it with perm; flag is
// os.O_TRUNC or os.O_APPEND.
func writeFile(root *os.Root, path string, b []byte, flag int, perm os.FileMode) error {
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|flag, perm)
	if err != nil {
		return err
	}
	// OpenFile only applies perm when it creates the file
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
