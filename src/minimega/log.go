// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"os"
	"runtime"
	"strconv"
)

var logCLIHandlers = []minicli.Handler{
	{ // log level
		HelpShort: "set or print the log level",
		HelpLong: `
Set the log level to one of [debug, info, warn, error, fatal]. Log levels
inherit lower levels, so setting the level to error will also log fatal, and
setting the mode to debug will log everything.`,
		Patterns: []string{
			"log level [debug,info,warn,error,fatal]",
			"clear log level",
		},
		Record: true,
		Call:   cliLogLevel,
	},
	{ // log stderr
		HelpShort: "enable or disable logging to stderr",
		HelpLong:  "enable or disable logging to stderr",
		Patterns: []string{
			"log stderr [true,false]",
			"clear log stderr",
		},
		Record: true,
		Call:   cliLogStderr,
	},
	{ // log file
		HelpShort: "enable logging to a file",
		HelpLong: `
Log to a file. To disable file logging, call "clear log file".`,
		Patterns: []string{
			"log file <file>",
			"clear log file",
		},
		Record: true,
		Call:   cliLogFile,
	},
}

func init() {
	registerHandlers("log", logCLIHandlers)
}

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

func cliLogLevel(c *minicli.Command) minicli.Responses {
	resp := &minicli.Response{Host: hostname}

	if isClearCommand(c) {
		// Reset the level to default
		*f_loglevel = "error"
		log.SetLevel("stdio", log.ERROR)
		log.SetLevel("file", log.ERROR)
	} else if len(c.BoolArgs) == 0 {
		// Print the level
		resp.Response = *f_loglevel
	} else {
		// Bool args should only have a single key that is the log level
		for k := range c.BoolArgs {
			level, err := log.LevelInt(k)
			if err != nil {
				panic("someone goofed on the patterns")
			}

			*f_loglevel = k
			// forget the error, if they don't exist we shouldn't be setting
			// their level, so we're fine.
			log.SetLevel("stdio", level)
			log.SetLevel("file", level)
		}
	}

	return minicli.Responses{resp}
}

func cliLogStderr(c *minicli.Command) minicli.Responses {
	resp := &minicli.Response{Host: hostname}

	if isClearCommand(c) || c.BoolArgs["false"] {
		// Turn off logging to stderr
		log.DelLogger("stdio")
	} else if len(c.BoolArgs) == 0 {
		// Print true or false depending on whether stderr is enabled
		_, err := log.GetLevel("stdio")
		resp.Response = strconv.FormatBool(err == nil)
	} else if c.BoolArgs["true"] {
		// Enable stderr logging or adjust the level if already enabled
		level, _ := log.LevelInt(*f_loglevel)
		_, err := log.GetLevel("stdio")
		if err != nil {
			log.AddLogger("stdio", os.Stderr, level, true)
		} else {
			// TODO: Why do this? cliLogLevel updates stdio level whenever
			// f_loglevel is changed.
			log.SetLevel("stdio", level)
		}
	}

	return minicli.Responses{resp}
}

func cliLogFile(c *minicli.Command) minicli.Responses {
	resp := &minicli.Response{Host: hostname}

	// TODO: In the old implementation, if the provided file was "false" we
	// would disable logging to file. This wasn't documented in the help text
	// and, therefore, it's not implemented here. Double check with Fritz about
	// this change.
	if isClearCommand(c) {
		// Turn of logging to file
		log.DelLogger("file")
	} else if len(c.StringArgs) == 0 {
		// Print true or false depending on whether file is enabled
		_, err := log.GetLevel("file")
		resp.Response = strconv.FormatBool(err == nil)
	} else {
		// Enable logging to file if it's not already enabled
		level, _ := log.LevelInt(*f_loglevel)

		// TODO: What closes the file?
		logfile, err := os.OpenFile(c.StringArgs["file"], os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			resp.Error = err.Error()
		} else {
			log.AddLogger("file", logfile, level, false)
		}
	}

	return minicli.Responses{resp}
}
