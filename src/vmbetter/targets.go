// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
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
// cp, and extlinux.
func Buildqcow2(buildPath string, c vmconfig.Config) error {
	targetName := strings.Split(filepath.Base(c.Path), ".")[0]
	log.Debugln("using target name:", targetName)

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Final qcow2 target
	targetqcow2 := fmt.Sprintf("%v/%v.qcow2", wd, targetName)
	// Temporary file for building qcow2 file, will be renamed to targetqcow2
	tmpqcow2 := fmt.Sprintf("%v/%v.qcow2.tmp", wd, targetName)

	err = createQcow2(tmpqcow2, *f_qcowsize)
	if err != nil {
		return err
	}

	// Cleanup our temporary building file
	defer func() {
		// Check if file exists
		if _, err := os.Stat(tmpqcow2); err == nil {
			if err = os.Remove(tmpqcow2); err != nil {
				log.Errorln(err)
			}
		}
	}()

	dev, err := nbdConnectQcow2(tmpqcow2)
	if err != nil {
		return err
	}

	// Disconnect from the nbd device
	defer func() {
		if err := nbdDisconnectQcow2(dev); err != nil {
			log.Errorln(err)
		}
	}()

	if err := partitionQcow2(dev); err != nil {
		return err
	}

	if err := formatQcow2(dev + "p1"); err != nil {
		return err
	}

	mountPath, err := mountQcow2(dev + "p1")
	if err != nil {
		return err
	}

	if err := copyQcow2(buildPath, mountPath); err != nil {
		if err := umountQcow2(mountPath); err != nil {
			log.Errorln(err)
		}
		return err
	}

	if err := extlinux(mountPath); err != nil {
		if err := umountQcow2(mountPath); err != nil {
			log.Errorln(err)
		}
		return err
	}

	if err := umountQcow2(mountPath); err != nil {
		return err
	}

	if err := extlinuxMBR(dev, *f_mbr); err != nil {
		return err
	}

	return os.Rename(tmpqcow2, targetqcow2)
}

// createQcow2 creates a target qcow2 image using qemu-img. Size specifies the
// size of the image in bytes but optional suffixes such as "K" and "G" can be
// used. See qemu-img(8) for details.
func createQcow2(target, size string) error {
	// create our qcow image
	p := process("qemu-img")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"create",
			"-f",
			"qcow2",
			target,
			size,
		},
		Env: nil,
		Dir: "",
	}
	log.Debug("creating disk image with cmd: %v", cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	log.LogAll(stdout, log.INFO, "qemu-img")
	log.LogAll(stderr, log.ERROR, "qemu-img")

	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// nbdConnectQcow2 exports a target image using the NBD protocol using the
// qemu-nbd. If successful, returns the NBD device.
func nbdConnectQcow2(target string) (string, error) {
	// Find the first available nbd, there is a race condition here.
	nbdPath := ""

	// Have 128 as the upper bound as it used to be the arbitrary limit on the
	// number of NBD devices.
	for i := 0; i < 128; i++ {
		path := fmt.Sprintf("/dev/nbd%d", i)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			nbdPath = path
		}
	}

	if nbdPath == "" {
		return "", errors.New("unable to find available nbd device")
	}

	// connect it to qemu-nbd
	p := process("qemu-nbd")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-c",
			nbdPath,
			target,
		},
		Env: nil,
		Dir: "",
	}
	log.Debug("connecting to nbd with cmd: %v", cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	log.LogAll(stdout, log.INFO, "qemu-nbd")
	log.LogAll(stderr, log.ERROR, "qemu-nbd")

	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return nbdPath, nil
}

// partitionQcow2 partitions the provided device creating one primary partition
// that is the size of the whole device and bootable.
func partitionQcow2(dev string) error {
	// partition with fdisk
	p := process("fdisk")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			dev,
		},
		Env: nil,
		Dir: "",
	}
	sIn, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	log.LogAll(stdout, log.INFO, "fdisk")
	log.LogAll(stderr, log.INFO, "fdisk")

	log.Debug("partitioning with cmd: %v", cmd)
	err = cmd.Start()
	if err != nil {
		return err
	}
	io.WriteString(sIn, "n\np\n1\n\n\na\n1\nw\n")
	err = cmd.Wait()
	if err != nil {
		return err
	}
	return nil
}

// formatQcow2 formats a partition with the default linux filesystem type.
func formatQcow2(dev string) error {
	// make an ext4 filesystem
	p := process("mkfs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			dev,
		},
		Env: nil,
		Dir: "",
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	log.LogAll(stdout, log.INFO, "mkfs")
	log.LogAll(stderr, log.INFO, "mkfs")

	log.Debug("formatting with with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// mountQcow2 mounts a partition to a temporary directory. If successful,
// returns the path to that temporary directory.
func mountQcow2(dev string) (string, error) {
	// mount the filesystem
	mountPath, err := ioutil.TempDir("", "vmbetter_mount_")
	if err != nil {
		log.Fatalln("cannot create temporary directory:", err)
	}
	log.Debugln("using mount path:", mountPath)
	p := process("mount")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			dev,
			mountPath,
		},
		Env: nil,
		Dir: "",
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	log.LogAll(stdout, log.INFO, "mount")
	log.LogAll(stderr, log.ERROR, "mount")

	log.Debug("mounting with with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return mountPath, nil
}

// copyQcow2 recursively copies files from src to dst using cp.
func copyQcow2(src, dst string) error {
	// copy everything over
	p := process("cp")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-a",
			"-v",
			src + "/.",
			dst,
		},
		Env: nil,
		Dir: "",
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	log.LogAll(stdout, log.INFO, "cp")
	log.LogAll(stderr, log.ERROR, "cp")

	log.Debug("copy with with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// extlinux installs the SYSLINUX bootloader using extlinux. Path should be the
// root directory for the filesystem. extlinux also writes out a
// minimega-specific configuration file for SYSLINUX.
func extlinux(path string) error {
	// install extlinux
	p := process("extlinux")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"--install",
			path + "/boot",
		},
		Env: nil,
		Dir: "",
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	log.LogAll(stdout, log.INFO, "extlinux")
	log.LogAll(stderr, log.INFO, "extlinux")

	log.Debug("installing bootloader with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		return err
	}

	// write out the bootloader config, but first figure out the kernel and
	// initrd files in /boot
	filepath.Walk(path+"/boot", kernelWalker)
	if kernelName == "" {
		return fmt.Errorf("could not find kernel name")
	}
	if initrdName == "" {
		return fmt.Errorf("could not find initrd name")
	}

	extlinuxConfig := fmt.Sprintf("DEFAULT minimegalinux\nLABEL minimegalinux\nSAY booting minimegalinux\nLINUX /boot/%v\nAPPEND root=/dev/sda1\nINITRD /boot/%v", kernelName, initrdName)

	err = ioutil.WriteFile(path+"/boot/extlinux.conf", []byte(extlinuxConfig), os.FileMode(0660))
	if err != nil {
		return err
	}
	return nil
}

// umountQcow2 unmounts qcow2 image that was previously mounted with
// mountQcow2.
func umountQcow2(path string) error {
	// unmount
	p := process("umount")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			path,
		},
		Env: nil,
		Dir: "",
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	log.LogAll(stdout, log.INFO, "umount")
	log.LogAll(stderr, log.ERROR, "umount")

	log.Debug("unmounting with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// extlinuxMBR installs the specified master boot record in the partition table
// for the provided device.
func extlinuxMBR(dev, mbr string) error {
	// dd the mbr image
	p := process("dd")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			fmt.Sprintf("if=%v", mbr),
			"conv=notrunc",
			"bs=440",
			"count=1",
			fmt.Sprintf("of=%v", dev),
		},
		Env: nil,
		Dir: "",
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	log.LogAll(stdout, log.INFO, "dd")
	log.LogAll(stderr, log.INFO, "dd")

	log.Debug("installing mbr with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// nbdDisconnectQcow2 disconnects a given NBD using qemu-nbd.
func nbdDisconnectQcow2(dev string) error {
	// disconnect nbd
	p := process("qemu-nbd")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-d",
			dev,
		},
		Env: nil,
		Dir: "",
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	log.LogAll(stdout, log.INFO, "qemu-nbd")
	log.LogAll(stderr, log.ERROR, "qemu-nbd")

	log.Debug("disconnecting nbd with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		return err
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
