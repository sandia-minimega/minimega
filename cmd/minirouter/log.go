// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"strings"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	LOG_TAG_SIZE = 20
)

func logSetupPushUp() {
	// a special logger for pushing logs up to minimega
	if *f_miniccc != "" && *f_u == "" {
		var f tagLogger
		log.AddLogger("taglogger", &f, log.LevelFlag, false)
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
			"log level <debug,info,warn,error,fatal>",
		},
		Call: handleLog,
	})
}

func handleLog(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()

	// search for level in BoolArgs, we know that one of the BoolArgs will
	// parse without error thanks to minicli.
	for k := range c.BoolArgs {
		v, err := log.ParseLevel(k)
		if err == nil {
			log.SetLevelAll(v)
			return
		}
	}

	// unreachable
}
