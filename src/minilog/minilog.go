// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>
// Devin Cook <devcook@sandia.gov>

// minilog extends Go's logging functionality to allow for multiple
// loggers, each one with their own logging level. To use minilog, call
// AddLogger() to set up each desired logger, then use the package-level
// logging functions defined to send messages to all defined loggers.
package minilog

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	golog "log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	loggers = make(map[string]*minilogger)
	logLock sync.RWMutex
)

var (
	colorLine  = FgYellow
	colorDebug = FgBlue
	colorInfo  = FgGreen
	colorWarn  = FgYellow
	colorError = FgRed
	colorFatal = FgRed
)

const DefaultLevel = ERROR

var (
	LevelFlag   = DefaultLevel
	VerboseFlag bool
	FileFlag    string
)

func init() {
	// Add non-builtin flag type
	flag.Var(&LevelFlag, "level", "set log level: [debug, info, warn, error, fatal]")
	flag.BoolVar(&VerboseFlag, "v", true, "log on stderr")
	flag.BoolVar(&VerboseFlag, "verbose", true, "log on stderr")
	flag.StringVar(&FileFlag, "logfile", "", "specify file to log to")
}

// Adds a logger set to log only events at level specified or higher.
// output: io.Writer instance to which to log (can be os.Stderr or os.Stdout)
// level:  one of the minilogging levels defined as a constant
func AddLogger(name string, output io.Writer, level Level, color bool) {
	logLock.Lock()
	defer logLock.Unlock()

	loggers[name] = &minilogger{golog.New(output, "", golog.LstdFlags), level, color, nil}
}

func AddLogRing(name string, l *Ring, level Level) {
	logLock.Lock()
	defer logLock.Unlock()

	loggers[name] = &minilogger{
		logger: l,
		Level:  level,
	}
}

// Remove a named logger that was added using AddLogger
func DelLogger(name string) {
	logLock.Lock()
	defer logLock.Unlock()

	delete(loggers, name)
}

func Loggers() []string {
	logLock.Lock()
	defer logLock.Unlock()

	var ret []string
	for k, _ := range loggers {
		ret = append(ret, k)
	}
	return ret
}

// WillLog returns true if logging to a specific log level will result in
// actual logging. Useful if the logging text itself is expensive to produce.
func WillLog(level Level) bool {
	logLock.Lock()
	defer logLock.Unlock()

	for _, v := range loggers {
		if v.Level <= level {
			return true
		}
	}
	return false
}

// Change a log level for a named logger.
func SetLevel(name string, level Level) error {
	logLock.Lock()
	defer logLock.Unlock()

	if loggers[name] == nil {
		return errors.New("logger does not exist")
	}
	loggers[name].Level = level
	return nil
}

// SetLevelAll changes the log level for all loggers to the provided level
func SetLevelAll(level Level) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		logger.Level = level
	}
}

// Return the log level for a named logger.
func GetLevel(name string) (Level, error) {
	logLock.Lock()
	defer logLock.Unlock()

	if loggers[name] == nil {
		return -1, errors.New("logger does not exist")
	}
	return loggers[name].Level, nil
}

// Log all input from an io.Reader, splitting on lines, until EOF. LogAll
// starts a goroutine and returns immediately.
func LogAll(i io.Reader, level Level, name string) {
	go func(i io.Reader, level Level, name string) {
		r := bufio.NewReader(i)
		for {
			d, err := r.ReadString('\n')
			if d := strings.TrimSpace(d); d != "" {
				log(level, name, d)
			}
			if level == FATAL {
				os.Exit(1)
			}
			if err != nil {
				break
			}
		}
	}(i, level, name)
}

// Setup log according to flags and OS.
// Replaces the logSetup() that each package used to have.
func Init() {
	color := true
	if runtime.GOOS == "windows" {
		color = false
	}

	if VerboseFlag {
		AddLogger("stderr", os.Stderr, LevelFlag, color)
	}

	if FileFlag != "" {
		err := os.MkdirAll(filepath.Dir(FileFlag), 0755)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logfile, err := os.OpenFile(FileFlag, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		AddLogger("file", logfile, LevelFlag, false)
	}
}

func Filters(name string) ([]string, error) {
	logLock.Lock()
	defer logLock.Unlock()

	if l, ok := loggers[name]; ok {
		var ret = make([]string, len(l.filters))
		copy(ret, l.filters)
		return ret, nil
	} else {
		return nil, fmt.Errorf("no such logger %v", name)
	}
}

func AddFilter(name string, filter string) error {
	logLock.Lock()
	defer logLock.Unlock()

	if l, ok := loggers[name]; ok {
		for _, f := range l.filters {
			if f == filter {
				return nil
			}
		}
		l.filters = append(l.filters, filter)
	} else {
		return fmt.Errorf("no such logger %v", name)
	}
	return nil
}

func DelFilter(name string, filter string) error {
	logLock.Lock()
	defer logLock.Unlock()

	if l, ok := loggers[name]; ok {
		for i, f := range l.filters {
			if f == filter {
				l.filters = append(l.filters[:i], l.filters[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("filter %v does not exist", filter)
	} else {
		return fmt.Errorf("no such logger %v", name)
	}
}

func log(level Level, name, format string, arg ...interface{}) {
	logLock.RLock()
	defer logLock.RUnlock()

	for _, logger := range loggers {
		if logger.Level <= level {
			logger.log(level, name, format, arg...)
		}
	}
}

func logln(level Level, name string, arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= level {
			logger.logln(level, name, arg...)
		}
	}
}

func Debug(format string, arg ...interface{}) {
	log(DEBUG, "", format, arg...)
}

func Info(format string, arg ...interface{}) {
	log(INFO, "", format, arg...)
}

func Warn(format string, arg ...interface{}) {
	log(WARN, "", format, arg...)
}

func Error(format string, arg ...interface{}) {
	log(ERROR, "", format, arg...)
}

func Fatal(format string, arg ...interface{}) {
	log(FATAL, "", format, arg...)

	os.Exit(1)
}

func Debugln(arg ...interface{}) {
	logln(DEBUG, "", arg...)
}

func Infoln(arg ...interface{}) {
	logln(INFO, "", arg...)
}

func Warnln(arg ...interface{}) {
	logln(WARN, "", arg...)
}

func Errorln(arg ...interface{}) {
	logln(ERROR, "", arg...)
}

func Fatalln(arg ...interface{}) {
	logln(FATAL, "", arg...)

	os.Exit(1)
}
