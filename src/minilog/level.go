// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package minilog

import (
	"errors"
	"fmt"
)

type Level int

// Log levels supported:
// DEBUG -> INFO -> WARN -> ERROR -> FATAL
const (
	_ Level = iota
	DEBUG
	INFO
	WARN
	ERROR
	FATAL
)

// ParseLevel returns the log level from a string.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "debug":
		return DEBUG, nil
	case "info":
		return INFO, nil
	case "warn":
		return WARN, nil
	case "error":
		return ERROR, nil
	case "fatal":
		return FATAL, nil
	}
	return -1, errors.New("invalid log level")
}

func (l *Level) Set(s string) (err error) {
	*l, err = ParseLevel(s)
	return
}

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "debug"
	case INFO:
		return "info"
	case WARN:
		return "warn"
	case ERROR:
		return "error"
	case FATAL:
		return "fatal"
	}

	return fmt.Sprintf("Level(%d)", l)
}
