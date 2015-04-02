// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"minicli"
	log "minilog"
	"os"
	"strconv"
)

var (
	logFile *os.File
)

var logCLIHandlers = []minicli.Handler{
	{ // log level
		HelpShort: "set or print the log level",
		HelpLong: `
Set the log level to one of [debug,info,warn,error,fatal]. Log levels inherit
lower levels, so setting the level to error will also log fatal, and setting
the mode to debug will log everything.`,
		Patterns: []string{
			"log level [debug,info,warn,error,fatal]",
		},
		Call: wrapSimpleCLI(cliLogLevel),
	},
	{ // log stderr
		HelpShort: "enable or disable logging to stderr",
		Patterns: []string{
			"log stderr [true,false]",
		},
		Call: wrapSimpleCLI(cliLogStderr),
	},
	{ // log file
		HelpShort: "enable logging to a file",
		HelpLong: `
Log to a file. To disable file logging, call "clear log file".`,
		Patterns: []string{
			"log file [file]",
		},
		Call: wrapSimpleCLI(cliLogFile),
	},
	{ // clear log
		HelpShort: "reset state for logging",
		HelpLong: `
Resets state for logging. See "help log ..." for more information.`,
		Patterns: []string{
			"clear log",
			"clear log <file,>",
			"clear log <level,>",
			"clear log <stderr,>",
		},
		Call: wrapSimpleCLI(cliLogClear),
	},
}

func init() {
	registerHandlers("log", logCLIHandlers)
}

func cliLogLevel(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if len(c.BoolArgs) == 0 {
		// Print the level
		resp.Response = *f_loglevel
	} else {
		// Bool args should only have a single key that is the log level
		for k := range c.BoolArgs {
			level, err := log.LevelInt(k)
			if err != nil {
				log.Fatalln("someone goofed on the patterns")
			}

			*f_loglevel = k
			// forget the error, if they don't exist we shouldn't be setting
			// their level, so we're fine.
			log.SetLevel("stdio", level)
			log.SetLevel("file", level)
		}
	}

	return resp
}

func cliLogStderr(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["false"] {
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

	return resp
}

func cliLogFile(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if len(c.StringArgs) == 0 {
		// Print true or false depending on whether file is enabled
		if logFile != nil {
			resp.Response = logFile.Name()
		}
	} else {
		var err error

		// Enable logging to file if it's not already enabled
		level, _ := log.LevelInt(*f_loglevel)

		if logFile != nil {
			if err = stopFileLogger(); err != nil {
				resp.Error = err.Error()
				return resp
			}
		}

		flags := os.O_WRONLY | os.O_APPEND | os.O_CREATE
		logFile, err = os.OpenFile(c.StringArgs["file"], flags, 0660)
		if err != nil {
			resp.Error = err.Error()
		} else {
			log.AddLogger("file", logFile, level, false)
		}
	}

	return resp
}

func cliLogClear(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	// Reset file if explicitly cleared or we're clearing everything
	if c.BoolArgs["file"] || len(c.BoolArgs) == 0 {
		if err := stopFileLogger(); err != nil {
			resp.Error = err.Error()
			return resp
		}
	}

	// Reset level if explicitly cleared or we're clearing everything
	if c.BoolArgs["level"] || len(c.BoolArgs) == 0 {
		// Reset to default level
		*f_loglevel = "error"
		log.SetLevel("stdio", log.ERROR)
		log.SetLevel("file", log.ERROR)
	}

	// Reset stderr if explicitly cleared or we're clearing everything
	if c.BoolArgs["stderr"] || len(c.BoolArgs) == 0 {
		// Delete logger to stdout
		log.DelLogger("stdio")
	}

	return resp
}

// stopFileLogger gets rid of the previous file logger
func stopFileLogger() error {
	log.DelLogger("file")

	err := logFile.Close()
	if err != nil {
		log.Error("error closing log file: %v", err)
	} else {
		logFile = nil
	}

	return err
}
