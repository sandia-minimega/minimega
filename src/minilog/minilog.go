// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>
// Devin Cook <devcook@sandia.gov>
//
// This package extends Go's logging functionality to allow for multiple
// loggers, each one with their own logging level. To use minilog, call
// AddLogger() to set up each desired logger, then use the package-level
// logging functions defined to send messages to all defined loggers.

package minilog

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"bufio"
)

// Log levels supported:
// DEBUG -> INFO -> WARN -> ERROR -> FATAL
const (
	DEBUG = iota
	INFO
	WARN
	ERROR
	FATAL
)

var (
	loggers     map[string]*minilogger
	color_line  = FgYellow
	color_debug = FgBlue
	color_info  = FgGreen
	color_warn  = FgYellow
	color_error = FgRed
	color_fatal = FgRed
)

type minilogger struct {
	*log.Logger
	Level int
	Color bool // print in color
}

func init() {
	loggers = make(map[string]*minilogger)
}

// Adds a logger set to log only events at level specified or higher.
// output: io.Writer instance to which to log (can be os.Stderr or os.Stdout)
// level:  one of the minilogging levels defined as a constant
func AddLogger(name string, output io.Writer, level int, color bool) {
	loggers[name] = &minilogger{log.New(output, "", log.LstdFlags), level, color}
}

func DelLogger(name string) {
	delete(loggers, name)
}

func SetLevel(name string, level int) error {
	if loggers[name] == nil {
		return errors.New("logger does not exist")
	}
	loggers[name].Level = level
	return nil
}

func GetLevel(name string) (int, error) {
	if loggers[name] == nil {
		return -1, errors.New("logger does not exist")
	}
	return loggers[name].Level, nil
}

// Log all input from an io.Reader, splitting on lines, until EOF. LogAll starts a goroutine and 
// returns immediately.
func LogAll(i io.Reader, level int) {
	go func() {
		r := bufio.NewReader(i)
		for {
			d, err := r.ReadString('\n')
			if err != nil {
				break
			}
			for _, l := range loggers {
				l.logln(level, d)
			}
		}
	}()
}

// Return the log level from a string. Useful for parsing log levels from a flag package.
func LevelInt(l string) (int, error) {
	switch l {
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

func (l *minilogger) prologue(level int) (msg string) {
	switch level {
	case DEBUG:
		msg += "DEBUG "
	case INFO:
		msg += "INFO "
	case WARN:
		msg += "WARN "
	case ERROR:
		msg += "ERROR "
	default:
		msg += "FATAL "
	}

	_, file, line, _ := runtime.Caller(3)
	short := file
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			short = file[i+1:]
			break
		}
	}

	msg += short + ":" + strconv.Itoa(line) + ": "

	if l.Color {
		msg = color_line + msg
		switch level {
		case DEBUG:
			msg += color_debug
		case INFO:
			msg += color_info
		case WARN:
			msg += color_warn
		case ERROR:
			msg += color_error
		default:
			msg += color_fatal
		}
	}
	return
}

func (l *minilogger) epilogue() string {
	if l.Color {
		return Reset
	}
	return ""
}

func (l *minilogger) log(level int, format string, arg ...interface{}) {
	msg := l.prologue(level) + fmt.Sprintf(format, arg...) + l.epilogue()
	l.Print(msg)
}

func (l *minilogger) logln(level int, arg ...interface{}) {
	msg := l.prologue(level) + fmt.Sprintln(arg...) + l.epilogue()
	l.Println(msg)
}

func Debug(format string, arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= DEBUG {
			logger.log(DEBUG, format, arg...)
		}
	}
}

func Info(format string, arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= INFO {
			logger.log(INFO, format, arg...)
		}
	}
}

func Warn(format string, arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= WARN {
			logger.log(WARN, format, arg...)
		}
	}
}

func Error(format string, arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= ERROR {
			logger.log(ERROR, format, arg...)
		}
	}
}

func Fatal(format string, arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= FATAL {
			logger.log(FATAL, format, arg...)
		}
	}
	os.Exit(1)
}

func Debugln(arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= DEBUG {
			logger.logln(DEBUG, arg...)
		}
	}
}

func Infoln(arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= INFO {
			logger.logln(INFO, arg...)
		}
	}
}

func Warnln(arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= WARN {
			logger.logln(WARN, arg...)
		}
	}
}

func Errorln(arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= ERROR {
			logger.logln(ERROR, arg...)
		}
	}
}

func Fatalln(arg ...interface{}) {
	for _, logger := range loggers {
		if logger.Level <= FATAL {
			logger.logln(FATAL, arg...)
		}
	}
	os.Exit(1)
}
