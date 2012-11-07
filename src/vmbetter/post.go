package main

import (
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"vmconfig"
)

// post_build_commands invokes any commands listed in the postbuild variable
// of a config file. It does so by copying the entire string of the postbuild
// variable into a bash script under /tmp of the build directory, and then 
// executing it with bash inside of a chroot. Post build commands are executed
// in depth-first order.
func post_build_commands(build_path string, c vmconfig.Config) error {
	for _, p := range c.Postbuilds {
		log.Debugln("postbuild:", p)

		tmpfile := build_path + "/tmp/postbuild.bash"

		ioutil.WriteFile(tmpfile, []byte(p), 0770)

		cmd := exec.Command("chroot", build_path, "/bin/bash", "/tmp/postbuild.bash")
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
