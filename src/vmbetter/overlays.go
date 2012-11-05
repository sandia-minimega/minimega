package main

import (
	log "minilog"
	"os/exec"
	"vmconfig"
)

func overlays(build_path string, c vmconfig.Config) error {
	// copy the overlays in order
	for _, o := range c.Overlays {
		log.Infoln("copying overlay:", o)

		cmd := exec.Command("cp", "-r", "-v", o+"/.", build_path)
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
	}
	return nil
}
