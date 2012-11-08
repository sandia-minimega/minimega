package main

import (
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"vmconfig"
)

// BuildTargets generates the initrd and kernel files as the last stage of the
// build process. It does so by writing a find/cpio/gzip command as a script
// to a temporary file and executing that in a bash shell. The output filenames
// are equal to the base name of the input config file. So a config called
// 'my_vm.conf' will generate 'my_vm.initrd' and 'my_vm.kernel'. The kernel
// image is the one found in /boot of the build directory.
func BuildTargets(build_path string, c vmconfig.Config) error {
	target_name := strings.Split(filepath.Base(c.Path), ".")[0]
	log.Debugln("using target name:", target_name)

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	target_initrd := fmt.Sprintf("%v/%v.initrd", wd, target_name)
	target_kernel := fmt.Sprintf("%v/%v.kernel", wd, target_name)

	f, err := ioutil.TempFile("", "vmbetter_cpio")
	if err != nil {
		return err
	}

	e_name := f.Name()
	initrd_command := fmt.Sprintf("cd %v && find . -print0 | cpio --quiet --null -ov --format=newc | gzip -9 > %v\ncp boot/vmlinu* %v", build_path, target_initrd, target_kernel)
	f.WriteString(initrd_command)
	f.Close()

	log.Debugln("initrd command:", initrd_command)

	cmd := exec.Command("bash", e_name)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	log.LogAll(stdout, log.INFO, "cpio")
	// BUG(fritz): the cpio command outputs regular stuff to stderr, so i have a hack to push all output to the INFO level, instead of INFO/ERROR
	log.LogAll(stderr, log.INFO, "cpio")

	err = cmd.Run()
	if err != nil {
		return err
	}
	os.Remove(e_name)

	return nil
}
