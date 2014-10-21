// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
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

	targetqcow2 := fmt.Sprintf("%v/%v.qcow2", wd, targetName)

	err = createQcow2(targetqcow2, *f_qcowsize)
	if err != nil {
		return err
	}

	dev, err := nbdConnectQcow2(targetqcow2)
	if err != nil {
		e2 := os.Remove(targetqcow2)
		if e2 != nil {
			log.Errorln(e2)
		}
		return err
	}

	err = partitionQcow2(dev)
	if err != nil {
		e2 := nbdDisconnectQcow2(dev)
		if e2 != nil {
			log.Errorln(e2)
		}
		e2 = os.Remove(targetqcow2)
		if e2 != nil {
			log.Errorln(e2)
		}
		return err
	}

	err = formatQcow2(dev + "p1")
	if err != nil {
		e2 := nbdDisconnectQcow2(dev)
		if e2 != nil {
			log.Errorln(e2)
		}
		e2 = os.Remove(targetqcow2)
		if e2 != nil {
			log.Errorln(e2)
		}
		return err
	}

	mountPath, err := mountQcow2(dev + "p1")
	if err != nil {
		e2 := nbdDisconnectQcow2(dev)
		if e2 != nil {
			log.Errorln(e2)
		}
		e2 = os.Remove(targetqcow2)
		if e2 != nil {
			log.Errorln(e2)
		}
		return err
	}

	err = copyQcow2(buildPath, mountPath)
	if err != nil {
		e2 := umountQcow2(mountPath)
		if e2 != nil {
			log.Errorln(e2)
		}
		e2 = nbdDisconnectQcow2(dev)
		if e2 != nil {
			log.Errorln(e2)
		}
		e2 = os.Remove(targetqcow2)
		if e2 != nil {
			log.Errorln(e2)
		}
		return err
	}

	err = extlinux(mountPath)
	if err != nil {
		e2 := umountQcow2(mountPath)
		if e2 != nil {
			log.Errorln(e2)
		}
		e2 = nbdDisconnectQcow2(dev)
		if e2 != nil {
			log.Errorln(e2)
		}
		e2 = os.Remove(targetqcow2)
		if e2 != nil {
			log.Errorln(e2)
		}
		return err
	}

	err = umountQcow2(mountPath)
	if err != nil {
		e2 := nbdDisconnectQcow2(dev)
		if e2 != nil {
			log.Errorln(e2)
		}
		e2 = os.Remove(targetqcow2)
		if e2 != nil {
			log.Errorln(e2)
		}
		return err
	}

	err = extlinuxMBR(dev)
	if err != nil {
		e2 := nbdDisconnectQcow2(dev)
		if e2 != nil {
			log.Errorln(e2)
		}
		e2 = os.Remove(targetqcow2)
		if e2 != nil {
			log.Errorln(e2)
		}
		return err
	}

	err = nbdDisconnectQcow2(dev)
	if err != nil {
		e2 := os.Remove(targetqcow2)
		if e2 != nil {
			log.Errorln(e2)
		}
		return err
	}

	return nil
}

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

func nbdConnectQcow2(target string) (string, error) {
	// connect it to qemu-nbd
	p := process("qemu-nbd")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-c",
			"/dev/nbd0",
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
	return "/dev/nbd0", nil
}

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

func extlinux(path string) error {
	// install extlinux
	p := process("extlinux")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"--install",
			filepath.Join(path, "/boot"),
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
	filepath.Walk(filepath.Join(path, "/boot"), kernelWalker)
	if kernelName == "" {
		return fmt.Errorf("could not find kernel name")
	}
	if initrdName == "" {
		return fmt.Errorf("could not find initrd name")
	}

	extlinuxConfig := fmt.Sprintf("DEFAULT minimegalinux\nLABEL minimegalinux\nSAY booting minimegalinux\nLINUX /boot/%v\nAPPEND root=/dev/sda1\nINITRD /boot/%v", kernelName, initrdName)

	err = ioutil.WriteFile(filepath.Join(path, "/boot/extlinux.conf"), []byte(extlinuxConfig), os.FileMode(0660))
	if err != nil {
		return err
	}
	return nil
}

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

func extlinuxMBR(dev string) error {
	// dd the mbr image
	p := process("dd")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			fmt.Sprintf("if=%v", *f_mbr),
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
