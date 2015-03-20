// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package nbd

import (
	"fmt"
	log "minilog"
	"os/exec"
)

var externalProcesses = map[string]string{
	"qemu-nbd": "qemu-nbd",
	"lsmod":    "lsmod",
	"modprobe": "modprobe",
}

// check for the presence of each of the external processes we may call,
// and error if any aren't in our path
func externalCheck() {
	for _, i := range externalProcesses {
		path, err := exec.LookPath(i)
		if err != nil {
			e := fmt.Sprintf("%v not found", i)
			log.Errorln(e)
		} else {
			log.Info("%v found at: %v", i, path)
		}
	}
}

func process(p string) string {
	path, err := exec.LookPath(externalProcesses[p])
	if err != nil {
		log.Errorln(err)
		return ""
	}
	return path
}
