// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	log "minilog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	LOG_TAG_SIZE = 10
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
		err := os.MkdirAll(filepath.Dir(*f_logfile), 0755)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logfile, err := os.OpenFile(*f_logfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		log.AddLogger("file", logfile, level, false)
	}

	// a special logger for pushing logs up to minimega
	if *f_miniccc != "" {
		f := tagLogger()
		log.AddLogger("taglogger", f, level, false)
	}
}

func tagLogger() io.Writer {
	var buf bytes.Buffer
	var lines []string

	go func() {
		scanner := bufio.NewScanner(&buf)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			if len(lines) > LOG_TAG_SIZE {
				lines = lines[1:]
			}
			output := strings.Join(lines, "\n")
			err := tag("minirouter_log", output)
			if err != nil {
				log.Errorln(err)
				break
			}
		}
		// we'll actually log our own errors and hope they show up in
		// another logger
		if err := scanner.Err(); err != nil {
			log.Errorln(err)
		}
	}()
	return &buf
}
