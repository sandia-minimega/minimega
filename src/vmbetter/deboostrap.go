package main

import (
	"fmt"
	log "minilog"
	"os/exec"
	"strings"
	"vmconfig"
)

// debootstrap will invoke the debootstrap tool with a target build directory
// in build_path, using configuration from c.
func debootstrap(build_path string, c vmconfig.Config) error {
	path, err := exec.LookPath("debootstrap")
	if err != nil {
		return fmt.Errorf("cannot find debootstrap: %v", err)
	}

	// build debootstrap parameters
	var args []string
	args = append(args, "--variant=minbase")
	args = append(args, fmt.Sprintf("--include=%v", strings.Join(c.Packages, ",")))
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
	log.LogAll(stdout, log.INFO, "debootstrap")
	log.LogAll(stderr, log.ERROR, "debootstrap")

	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
