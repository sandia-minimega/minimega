// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sandia-minimega/minimega/v2/internal/vmconfig"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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
			log.Debug("overlay directory '%v' does not exist as an absolute path or relative to the current working directory.", o)
			var path string
			base := filepath.Base(o)    // get base path of overlay directory
			if i == len(c.Overlays)-1 { // if this is the last overlay, we'll check relative to c.Path
				log.Debugln("non-parent overlay")
				path = filepath.Join(filepath.Dir(c.Path), base)
			} else { // if not, it's a parent overlay and we'll check relative to c.Parents[i]
				log.Debugln("parent overlay")
				path = filepath.Join(filepath.Dir(c.Parents[i]), base)
			}
			log.Debug("checking path relative to config location: '%v'", path)
			if _, err := os.Stat(path); os.IsNotExist(err) { // check if we can find overlay relative to config file
				return err // nope
			} else { // yep
				sourcePath = path
			}
		} else {
			sourcePath = o
		}

		p := process("cp")
		cmd := exec.Command(p, "-rvL", sourcePath+"/.", buildPath)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}
		log.LogAll(stdout, log.DEBUG, "cp")
		log.LogAll(stderr, log.ERROR, "cp")

		err = cmd.Run()
		if err != nil {
			return err
		}
	}
	return nil
}
