// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"os"
	"os/exec"
	"time"
)

// processWrapper executes the given arg list and returns a combined
// stdout/stderr and any errors. processWrapper blocks until the process exits.
func processWrapper(args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty argument list")
	}

	log.Debug("running %v", args)

	start := time.Now()
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	stop := time.Now()

	log.Debug("cmd %v completed in %v", args[0], stop.Sub(start))

	if err != nil {
		log.Debug("error running %v: %v %v", args, err, string(out))
	}

	return string(out), err
}

// processWrapperEnv executes the given arg list and returns a
// combined stdout/stderr and any errors. Unlike processWrapper,
// processWrapperEnv allows the user to set additional environment
// variables for the command. Note that the set environment variables
// are added to those that already exist in the
// environment. processWrapperEnv blocks until the process exits.
func processWrapperEnv(env []string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty argument list")
	}

	log.Debug("running %v", args)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)

	start := time.Now()
	out, err := cmd.CombinedOutput()
	stop := time.Now()

	log.Debug("cmd %v completed in %v", args[0], stop.Sub(start))

	if err != nil {
		log.Debug("error running %v: %v %v", args, err, string(out))
	}

	return string(out), err
}
