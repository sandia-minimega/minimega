// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"vmconfig"
)

var (
	kernelName string
	initrdName string
)

// BuildTargets generates the initrd and kernel files as the last stage of the
// build process. It does so by writing a find/cpio/gzip command as a script
// to a temporary file and executing that in a bash shell. The output filenames
// are equal to the base name of the input config file. So a config called
// 'my_vm.conf' will generate 'my_vm.initrd' and 'my_vm.kernel'. The kernel
// image is the one found in /boot of the build directory.
func BuildTargets(buildPath string, c vmconfig.Config) error {
	targetName := strings.Split(filepath.Base(c.Path), ".")[0]
	log.Debugln("using target name:", targetName)

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	targetInitrd := fmt.Sprintf("%v/%v.initrd", wd, targetName)
	targetKernel := fmt.Sprintf("%v/%v.kernel", wd, targetName)

	f, err := ioutil.TempFile("", "vmbetter_cpio")
	if err != nil {
		return err
	}

	eName := f.Name()
	initrdCommand := fmt.Sprintf("cd %v && find . -print0 | cpio --quiet --null -ov --format=newc | gzip -9 > %v\ncp boot/vmlinu* %v", buildPath, targetInitrd, targetKernel)
	f.WriteString(initrdCommand)
	f.Close()

	log.Debugln("initrd command:", initrdCommand)

	cmd := exec.Command("bash", eName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	log.LogAll(stdout, log.INFO, "cpio")
	// the cpio command outputs regular stuff to stderr, so i have a hack to push all output to the INFO level, instead of INFO/ERROR
	log.LogAll(stderr, log.INFO, "cpio")

	err = cmd.Run()
	if err != nil {
		return err
	}
	os.Remove(eName)

	return nil
}

// Buildqcow2 creates a qcow2 image using qemu-img, qemu-nbd, fdisk, mkfs.ext4,
// cp, and extlinux. SHEESH
func Buildqcow2(buildPath string, c vmconfig.Config) error {
	// TODO(fritz): cleanup on error
	// TODO(fritz): use logall to get debug output during disk creation
	targetName := strings.Split(filepath.Base(c.Path), ".")[0]
	log.Debugln("using target name:", targetName)

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	targetqcow2 := fmt.Sprintf("%v/%v.qcow2", wd, targetName)

	// create our qcow image
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("qemu-img")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"create",
			"-f",
			"qcow2",
			targetqcow2,
			*f_qcowsize,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("creating disk image with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	// connect it to qemu-nbd
	p = process("qemu-nbd")
	sOut.Reset()
	sErr.Reset()
	// TODO(fritz): don't hardcode nbd0
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-c",
			"/dev/nbd0",
			targetqcow2,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("connecting to nbd with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	// partition with fdisk
	p = process("fdisk")
	sOut.Reset()
	sErr.Reset()
	// TODO(fritz): don't hardcode nbd0
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"/dev/nbd0",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	sIn, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	log.Debug("partitioning with cmd: %v", cmd)
	err = cmd.Start()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}
	io.WriteString(sIn, "n\np\n1\n\n\na\n1\nw\n")
	err = cmd.Wait()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	// make an ext4 filesystem
	p = process("mkfs")
	sOut.Reset()
	sErr.Reset()
	// TODO(fritz): don't hardcode nbd0
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"/dev/nbd0p1",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("formatting with with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	// mount the filesystem
	mountPath, err := ioutil.TempDir("", "vmbetter_mount_")
	if err != nil {
		log.Fatalln("cannot create temporary directory:", err)
	}
	log.Debugln("using mount path:", mountPath)
	p = process("mount")
	sOut.Reset()
	sErr.Reset()
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"/dev/nbd0p1",
			mountPath,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("mounting with with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	// copy everything over
	p = process("cp")
	sOut.Reset()
	sErr.Reset()
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-a",
			buildPath + "/.",
			mountPath,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("copy with with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	// install extlinux
	p = process("extlinux")
	sOut.Reset()
	sErr.Reset()
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"--install",
			mountPath + "/boot",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("installing bootloader with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	// write out the bootloader config, but first figure out the kernel and
	// initrd files in /boot
	filepath.Walk(buildPath+"/boot", kernelWalker)
	if kernelName == "" {
		return fmt.Errorf("could not find kernel name")
	}
	if initrdName == "" {
		return fmt.Errorf("could not find initrd name")
	}

	extlinuxConfig := fmt.Sprintf("DEFAULT minimegalinux\nLABEL minimegalinux\nSAY booting minimegalinux\nLINUX /boot/%v\nAPPEND root=/dev/sda1\nINITRD /boot/%v", kernelName, initrdName)

	err = ioutil.WriteFile(mountPath+"/boot/extlinux.conf", []byte(extlinuxConfig), os.FileMode(0660))
	if err != nil {
		return err
	}

	// unmount
	p = process("umount")
	sOut.Reset()
	sErr.Reset()
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			mountPath,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("unmounting with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	// dd the mbr image
	p = process("dd")
	sOut.Reset()
	sErr.Reset()
	// TODO(fritz): add flag for mbr.bin
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"if=/usr/lib/syslinux/mbr.bin",
			"conv=notrunc",
			"bs=440",
			"count=1",
			fmt.Sprintf("of=/dev/nbd0"),
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("installing mbr with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	// disconnect nbd
	p = process("qemu-nbd")
	sOut.Reset()
	sErr.Reset()
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-d",
			"/dev/nbd0",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("disconnecting nbd with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}

	return nil
}

func kernelWalker(path string, info os.FileInfo, err error) error {
	if strings.Contains(info.Name(), "vmlinuz") {
		kernelName = info.Name()
	}
	if strings.Contains(info.Name(), "initrd") {
		initrdName = info.Name()
	}
	return nil
}
