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
		log.AddLogger("file", logfile, level, false)
	}
}

func cliLogLevel(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: *f_loglevel,
		}
	} else if len(c.Args) > 1 {
		return cliResponse{
			Error: "log_level must be [debug, info, warn, error, fatal]",
		}
	} else {
		level, err := log.LevelInt(c.Args[0])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		*f_loglevel = c.Args[0]
		// forget the error, if they don't exist we shouldn't be setting their level, so we're fine.
		log.SetLevel("stdio", level)
		log.SetLevel("file", level)
	}
	return cliResponse{}
}

func cliLogStderr(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		_, err := log.GetLevel("stdio")
		if err != nil {
			return cliResponse{
				Response: "false",
			}
		} else {
			return cliResponse{
				Response: "true",
			}
		}
	} else if len(c.Args) > 1 {
		return cliResponse{
			Error: "log_stderr takes only one argument",
		}
	} else {
		_, err := log.GetLevel("stdio")
		switch c.Args[0] {
		case "true":
			level, _ := log.LevelInt(*f_loglevel)
			if err != nil {
				log.AddLogger("stdio", os.Stderr, level, true)
			} else {
				log.SetLevel("stdio", level)
			}
		case "false":
			if err == nil {
				log.DelLogger("stdio")
			}
		default:
			return cliResponse{
				Error: "log_stderr must be [true, false]",
			}
		}
	}
	return cliResponse{}
}

func cliLogFile(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		_, err := log.GetLevel("file")
		if err != nil {
			return cliResponse{
				Response: "false",
			}
		} else {
			return cliResponse{
				Response: "true",
			}
		}
	} else if len(c.Args) > 1 {
		return cliResponse{
			Error: "log_file takes only one argument",
		}
	} else {
		_, err := log.GetLevel("file")

		// special case, disabling file logging with "false"
		if err == nil && c.Args[0] == "false" {
			log.DelLogger("file")
		} else {
			logfile, err := os.OpenFile(c.Args[0], os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
			level, _ := log.LevelInt(*f_loglevel)
			log.AddLogger("file", logfile, level, false)
		}
	}
	return cliResponse{}
}
