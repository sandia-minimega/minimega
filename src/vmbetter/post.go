// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"vmconfig"
)

// PostBuildCommands invokes any commands listed in the postbuild variable
// of a config file. It does so by copying the entire string of the postbuild
// variable into a bash script under /tmp of the build directory, and then
// executing it with bash inside of a chroot. Post build commands are executed
// in depth-first order.
func PostBuildCommands(buildPath string, c vmconfig.Config) error {
	for _, pb := range c.Postbuilds {
		log.Debugln("postbuild:", pb)

		tmpfile := buildPath + "/tmp/postbuild.bash"

		ioutil.WriteFile(tmpfile, []byte(pb), 0770)

		p := process("chroot")
		cmd := exec.Command(p, buildPath, "/bin/bash", "/tmp/postbuild.bash")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}
		log.LogAll(stdout, log.INFO, "postbuild")
		log.LogAll(stderr, log.ERROR, "postbuild")

		err = cmd.Run()
		if err != nil {
			return err
		}
		os.Remove(tmpfile)
	}
	return nil
}
