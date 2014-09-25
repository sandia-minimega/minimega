// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"vmconfig"
)

// Overlays copies any overlay directories indicated in c into the build
// directory build_path. Overlays are copied in depth-first order, so that
// the oldest parent overlay data is copied in first. This allows a child
// to overwrite any overlay data created by a parent.
func Overlays(buildPath string, c vmconfig.Config) error {
	// copy the overlays in order
	for i, o := range c.Overlays {
		log.Infoln("copying overlay:", o)

		var sourcePath string
		// check if overlay exists as absolute path or relative to cwd
		if _, err := os.Stat(o); os.IsNotExist(err) {
			// it doesn't, so we'll check relative to config file
			var path string
			base := filepath.Base(o)    // get base path of overlay directory
			if i == len(c.Overlays)-1 { // if this is the last overlay, we'll check relative to c.Path
				path = filepath.Dir(c.Path) + "/" + base
			} else { // if not, it's a parent overlay and we'll check relative to c.Parents[i]
				path = filepath.Dir(c.Parents[i]) + "/" + base
			}
			if _, err := os.Stat(path); os.IsNotExist(err) { // check if we can find overlay relative to config file
				return err // nope
			} else { // yep
				sourcePath = path
			}
		} else {
			sourcePath = o
		}
		cmd := exec.Command("cp", "-r", "-v", sourcePath+"/.", buildPath)
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
