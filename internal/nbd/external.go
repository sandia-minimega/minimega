// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package nbd

import (
	"fmt"
	"os/exec"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var ExternalDependencies = []string{
	"lsmod",
	"modprobe",
	"qemu-nbd",
}

// processWrapper executes the given arg list and returns a combined
// stdout/stderr and any errors. processWrapper blocks until the process exits.
func processWrapper(args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty argument list")
	}

	start := time.Now()
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	stop := time.Now()
	log.Debug("cmd %v completed in %v", args[0], stop.Sub(start))

	return string(out), err
}
