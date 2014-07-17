// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"os"
	"runtime"
)

func logSetup() {
	level, err := log.LevelInt(*f_loglevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	color := true
	if runtime.GOOS == "windows" {
		color = false
	}

	if *f_log {
		log.AddLogger("stdio", os.Stderr, level, color)
	}

	if *f_logfile != "" {
		logfile, err := os.OpenFile(*f_logfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		log.AddLogger("file", logfile, level, color)
	}
}

func logChange(level string, file string) error {
	l, err := log.LevelInt(level)
	if err != nil {
		return err
	}

	log.SetLevel("stdio", l)
	log.SetLevel("file", l)

	_, err = log.GetLevel("file")
	if err == nil && file == "" {
		log.DelLogger("file")
	} else if file != "" {
		logfile, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			return err
		}
		log.AddLogger("file", logfile, l, false)
	}
	return nil
}
