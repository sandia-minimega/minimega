// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/sys/unix"
)

// fcWaitDev waits for a device node to appear. Tests replace it.
var fcWaitDev = func(dev string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, err := os.Stat(dev)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%v did not appear after %v: %v", dev, timeout, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// FCInject is one entry of vm config inject. Partition 0 is the whole disk.
type FCInject struct {
	Partition int
	Src, Dst  string
	Delete    bool
}

type FCInjects []FCInject

// args returns the vm config inject arguments of i.
func (i FCInject) args() []string {
	p := strconv.Itoa(i.Partition)
	if i.Delete {
		return []string{"delete", p, i.Dst}
	}
	return []string{p, i.Src + ":" + i.Dst}
}

func (c FCInjects) String() string {
	var parts []string
	for _, i := range c {
		parts = append(parts, strings.Join(i.args(), " "))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (c FCInjects) WriteConfig(w io.Writer) error {
	for _, i := range c {
		if _, err := fmt.Fprintf(w, "vm config inject %v\n", quoteJoin(i.args(), " ")); err != nil {
			return err
		}
	}

	return nil
}

func (vm *FirecrackerConfig) ReadFieldConfig(r io.Reader, field, namespace string, _ *VMConfig) error {
	if field != "inject" {
		return nil
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "vm config inject ") {
			continue
		}

		injects, err := fcParseInjects(fieldsQuoteEscape(`"`, strings.TrimPrefix(line, "vm config inject ")))
		if err != nil {
			return err
		}

		vm.Injects = append(vm.Injects, injects...)
	}

	return scanner.Err()
}

// fcParseInjects parses `<partition> <src:dst>...` or
// `delete <partition> <dst>...`.
func fcParseInjects(args []string) (FCInjects, error) {
	del := len(args) > 0 && args[0] == "delete"
	if del {
		args = args[1:]
	}

	if len(args) < 2 {
		if del {
			return nil, errors.New("expected delete <partition> <dst>...")
		}
		return nil, errors.New("expected <partition> <src:dst>...")
	}

	part, err := strconv.Atoi(args[0])
	if err != nil || part < 0 {
		return nil, fmt.Errorf("invalid partition %q: must be an integer that is 0 or more", args[0])
	}

	var res FCInjects
	for _, arg := range args[1:] {
		i := FCInject{Partition: part, Delete: del, Dst: arg}

		if !del {
			parts := strings.Split(arg, ":")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return nil, fmt.Errorf("malformed inject %q: expected src:dst", arg)
			}
			i.Src, i.Dst = parts[0], parts[1]
		}

		if _, err := fcInjectRel(i.Dst); err != nil {
			return nil, err
		}

		res = append(res, i)
	}

	return res, nil
}

// fcCheckInjects returns an error when injects cannot apply to the VM disks.
func fcCheckInjects(snapshot bool, injects FCInjects) error {
	if len(injects) > 0 && !snapshot {
		return errors.New("vm config inject needs snapshot true")
	}
	return nil
}

// fcInjectRel returns dst as a path relative to the root of the filesystem.
func fcInjectRel(dst string) (string, error) {
	rel := filepath.Clean(strings.TrimLeft(dst, "/"))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("inject destination %q is not in the filesystem of the disk", dst)
	}

	return rel, nil
}

// fcApplyInjects copies files into or deletes files from the disk at
// diskHostPath, a raw file or a block device.
func fcApplyInjects(diskHostPath string, injects FCInjects) (err error) {
	if len(injects) == 0 {
		return nil
	}

	var order []int
	byPart := map[int][]FCInject{}
	for _, i := range injects {
		if _, ok := byPart[i.Partition]; !ok {
			order = append(order, i.Partition)
		}
		byPart[i.Partition] = append(byPart[i.Partition], i)
	}

	out, err := fcRunCmd("losetup", "--find", "--show", "--partscan", diskHostPath)
	if err != nil {
		return fmt.Errorf("attach %v: %v: %v", diskHostPath, strings.TrimSpace(out), err)
	}
	loop := strings.TrimSpace(out)

	defer func() {
		if out, err2 := fcRunCmd("losetup", "-d", loop); err2 != nil {
			err = errors.Join(err, fmt.Errorf("detach %v: %v: %v", loop, strings.TrimSpace(out), err2))
		}
	}()

	for _, part := range order {
		if err := fcInjectPartition(loop, part, byPart[part]); err != nil {
			return fmt.Errorf("inject into %v: %v", diskHostPath, err)
		}
	}

	return nil
}

func fcInjectPartition(loop string, part int, injects []FCInject) (err error) {
	dev := loop
	if part > 0 {
		dev = fmt.Sprintf("%vp%d", loop, part)
	}

	if err := fcWaitDev(dev, 5*time.Second); err != nil {
		return fmt.Errorf("partition %d: %v", part, err)
	}

	mnt, err := os.MkdirTemp(*f_base, "fc-inject-")
	if err != nil {
		return err
	}
	defer func() {
		if err2 := os.Remove(mnt); err2 != nil {
			err = errors.Join(err, err2)
		}
	}()

	if out, err := fcRunCmd("mount", "-w", dev, mnt); err != nil {
		return fmt.Errorf("mount %v: %v: %v", dev, strings.TrimSpace(out), err)
	}
	defer func() {
		if out, err2 := fcRunCmd("umount", mnt); err2 != nil {
			err = errors.Join(err, fmt.Errorf("unmount %v: %v: %v", mnt, strings.TrimSpace(out), err2))
		}
	}()

	return fcInjectInto(mnt, injects)
}

// fcInjectInto applies injects to the filesystem at dir. No change reaches a
// path outside dir.
func fcInjectInto(dir string, injects []FCInject) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	for _, i := range injects {
		rel, err := fcInjectRel(i.Dst)
		if err != nil {
			return err
		}

		if i.Delete {
			if _, err := root.Lstat(rel); errors.Is(err, os.ErrNotExist) {
				log.Warn("inject: path does not exist to delete: %v", i.Dst)
				continue
			}
			if err := fcRootRemoveAll(root, rel); err != nil {
				return fmt.Errorf("delete %v: %v", i.Dst, err)
			}
			continue
		}

		src := i.Src
		if !filepath.IsAbs(src) {
			src = filepath.Join(*f_iomBase, src)
		}

		if err := fcInjectCopy(root, src, rel); err != nil {
			return fmt.Errorf("copy %v to %v: %v", i.Src, i.Dst, err)
		}
	}

	return nil
}

func fcInjectCopy(root *os.Root, src, rel string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}

	if dfi, err := root.Lstat(rel); err == nil && dfi.IsDir() {
		rel = filepath.Join(rel, filepath.Base(src))
	}

	if err := fcRootMkdirAll(root, filepath.Dir(rel)); err != nil {
		return err
	}

	return fcRootCopy(root, src, rel, fi)
}

func fcFileMode(fi os.FileInfo) os.FileMode {
	return fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
}

func fcRootCopy(root *os.Root, src, rel string, fi os.FileInfo) error {
	switch {
	case fi.IsDir():
		if err := root.Mkdir(rel, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}

		d, err := root.Open(rel)
		if err != nil {
			return err
		}
		dfi, err := d.Stat()
		if err == nil && !dfi.IsDir() {
			err = fmt.Errorf("%v is not a directory", rel)
		}
		if err == nil {
			err = d.Chmod(fcFileMode(fi))
		}
		if err2 := d.Close(); err == nil {
			err = err2
		}
		if err != nil {
			return err
		}

		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			efi, err := e.Info()
			if err != nil {
				return err
			}
			if err := fcRootCopy(root, filepath.Join(src, e.Name()), filepath.Join(rel, e.Name()), efi); err != nil {
				return err
			}
		}

		return nil
	case fi.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if err := fcRootReplace(root, rel); err != nil {
			return err
		}

		d, err := root.Open(filepath.Dir(rel))
		if err != nil {
			return err
		}
		defer d.Close()

		return unix.Symlinkat(target, int(d.Fd()), filepath.Base(rel))
	case fi.Mode().IsRegular():
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()

		if err := fcRootReplace(root, rel); err != nil {
			return err
		}

		out, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		if err := out.Chmod(fcFileMode(fi)); err != nil {
			out.Close()
			return err
		}

		return out.Close()
	}

	return fmt.Errorf("%v: unsupported file type %v", src, fi.Mode().Type())
}

// fcRootReplace removes a symlink at rel, so that a write replaces the link
// and does not follow it.
func fcRootReplace(root *os.Root, rel string) error {
	fi, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		return root.Remove(rel)
	}

	return nil
}

func fcRootMkdirAll(root *os.Root, rel string) error {
	if rel == "." || rel == "" {
		return nil
	}

	var p string
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		p = filepath.Join(p, part)

		if err := root.Mkdir(p, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}

		fi, err := root.Stat(p)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("%v is not a directory", p)
		}
	}

	return nil
}

func fcRootRemoveAll(root *os.Root, rel string) error {
	fi, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	if fi.IsDir() {
		d, err := root.Open(rel)
		if err != nil {
			return err
		}
		names, err := d.Readdirnames(-1)
		d.Close()
		if err != nil {
			return err
		}

		for _, name := range names {
			if err := fcRootRemoveAll(root, filepath.Join(rel, name)); err != nil {
				return err
			}
		}
	}

	return root.Remove(rel)
}
