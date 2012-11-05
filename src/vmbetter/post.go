package main

import (
	log "minilog"
	"vmconfig"
	"os/exec"
	"io/ioutil"
	"os"
)

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
		log.LogAll(stdout, log.INFO)
		log.LogAll(stderr, log.ERROR)

		err = cmd.Run()
		if err != nil {
			return err
		}
		os.Remove(tmpfile)
	}
	return nil
}
