// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"os"
	"strings"
)

const (
	LOG_TAG_SIZE = 20
)

func logSetupPushUp() {
	level, err := log.LevelInt(*log.FLogLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// a special logger for pushing logs up to minimega
	if *f_miniccc != "" && *f_u == "" {
		var f tagLogger
		log.AddLogger("taglogger", &f, level, false)
	}
}

type tagLogger struct {
	lines []string
}

func (t *tagLogger) Write(p []byte) (int, error) {
	t.lines = append(t.lines, string(p))
	if len(t.lines) > LOG_TAG_SIZE {
		t.lines = t.lines[1:]
	}
	output := strings.Join(t.lines, "")
	err := tag("minirouter_log", output)
	if err != nil {
		return len(p), err
	}
	return len(p), nil
}

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"log level <fatal,error,warn,info,debug>",
		},
		Call: handleLog,
	})
}

func handleLog(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()

	var level int
	if c.BoolArgs["fatal"] {
		level = log.FATAL
	} else if c.BoolArgs["error"] {
		level = log.ERROR
	} else if c.BoolArgs["warn"] {
		level = log.WARN
	} else if c.BoolArgs["info"] {
		level = log.INFO
	} else if c.BoolArgs["debug"] {
		level = log.DEBUG
	}

	loggers := log.Loggers()
	for _, l := range loggers {
		log.SetLevel(l, level)
	}
}
