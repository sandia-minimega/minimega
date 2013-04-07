package main

import (
	log "minilog"
	"os/exec"
	"vmconfig"
)

// Overlays copies any overlay directories indicated in c into the build 
// directory build_path. Overlays are copied in depth-first order, so that
// the oldest parent overlay data is copied in first. This allows a child
// to overwrite any overlay data created by a parent.
func Overlays(buildPath string, c vmconfig.Config) error {
	// copy the overlays in order
	for _, o := range c.Overlays {
		log.Infoln("copying overlay:", o)

		cmd := exec.Command("cp", "-r", "-v", o+"/.", buildPath)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}
		log.LogAll(stdout, log.INFO, "cp")
		log.LogAll(stderr, log.ERROR, "cp")

		err = cmd.Run()
		if err != nil {
			return err
		}
	}
	return nil
}
