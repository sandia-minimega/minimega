package main

import (
	log "minilog"
	"vmconfig"
	"os/exec"
	"strings"
	"fmt"
	"io/ioutil"
)

func debootstrap(c vmconfig.Config) error {
	path, err := exec.LookPath("debootstrap")
	if err != nil {
		return fmt.Errorf("cannot find debootstrap: %v", err)
	}

	build_path, err := ioutil.TempDir("", "vmbetter_build_")
	if err != nil {
		return fmt.Errorf("cannot create temporary directory: %v", err)
	}
	log.Debugln("using build path:", build_path)

	// build debootstrap parameters
	var args []string
	args = append(args, "--variant=minbase")
	args = append(args, fmt.Sprintf("--include=%v", strings.Join(c.Packages,",")))
	args = append(args, "testing")
	args = append(args, build_path)
	args = append(args, *f_debian_mirror)

	log.Debugln("args:", args)

	cmd := exec.Command(path, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	log.LogAll(stdout, log.INFO)
	log.LogAll(stderr, log.ERROR)

	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
