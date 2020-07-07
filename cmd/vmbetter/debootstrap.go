// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/sandia-minimega/minimega/v2/internal/vmconfig"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Debootstrap will invoke the debootstrap tool with a target build directory
// in build_path, using configuration from c.
func Debootstrap(buildPath string, c vmconfig.Config) error {
	p := process("debootstrap")

	// build debootstrap parameters
	var args []string
	if *f_dstrp_append != "" {
		args = append(args, strings.Split(*f_dstrp_append, " ")...)
	}
	args = append(args, "--variant=minbase")
	args = append(args, fmt.Sprintf("--include=%v", strings.Join(c.Packages, ",")))
	args = append(args, *f_branch)
	args = append(args, buildPath)
	args = append(args, *f_debian_mirror)

	log.Debugln("args:", args)

	cmd := exec.Command(p, args...)
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
