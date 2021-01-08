// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"os/exec"
	"strings"
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

func runAll(format string, args []string) error {
	r := DefaultRunner(func(s string) error {
		cmd := strings.Split(fmt.Sprintf(format, s), " ")
		_, err := processWrapper(cmd...)
		return err
	})

	return r.RunAll(args)
}
