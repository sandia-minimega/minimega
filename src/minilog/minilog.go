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
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	loggers = make(map[string]*minilogger)
	logLock sync.Mutex
)

var (
	colorLine  = FgYellow
	colorDebug = FgBlue
	colorInfo  = FgGreen
	colorWarn  = FgYellow
	colorError = FgRed
	colorFatal = FgRed
)

type minilogger struct {
	*log.Logger
	Level   int
	Color   bool // print in color
	filters []string
}

// Adds a logger set to log only events at level specified or higher.
// output: io.Writer instance to which to log (can be os.Stderr or os.Stdout)
// level:  one of the minilogging levels defined as a constant
func AddLogger(name string, output io.Writer, level int, color bool) {
	logLock.Lock()
	defer logLock.Unlock()

	loggers[name] = &minilogger{log.New(output, "", log.LstdFlags), level, color, nil}
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
func WillLog(level int) bool {
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
func SetLevel(name string, level int) error {
	logLock.Lock()
	defer logLock.Unlock()

	if loggers[name] == nil {
		return errors.New("logger does not exist")
	}
	loggers[name].Level = level
	return nil
}

// Return the log level for a named logger.
func GetLevel(name string) (int, error) {
	logLock.Lock()
	defer logLock.Unlock()

	if loggers[name] == nil {
		return -1, errors.New("logger does not exist")
	}
	return loggers[name].Level, nil
}

// Log all input from an io.Reader, splitting on lines, until EOF. LogAll starts a goroutine and
// returns immediately.
//
// TODO: How to lock properly for this?
func LogAll(i io.Reader, level int, name string) {
	go func(i io.Reader, level int, name string) {
		r := bufio.NewReader(i)
		for {
			d, err := r.ReadString('\n')
			d = strings.TrimSpace(d)
			if d != "" {
				for _, logger := range loggers {
					if logger.Level <= level {
						msg := logger.prologue(level, name) + d + logger.epilogue()
						logger.Println(msg)
					}
				}
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

func (l *minilogger) prologue(level int, name string) (msg string) {
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

	if name == "" {
		_, file, line, _ := runtime.Caller(3)
		short := file
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				short = file[i+1:]
				break
			}
		}
		msg += short + ":" + strconv.Itoa(line) + ": "
	} else {
		msg += name + ": "
	}

	if l.Color {
		msg = colorLine + msg
		switch level {
		case DEBUG:
			msg += colorDebug
		case INFO:
			msg += colorInfo
		case WARN:
			msg += colorWarn
		case ERROR:
			msg += colorError
		default:
			msg += colorFatal
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
	msg := l.prologue(level, "") + fmt.Sprintf(format, arg...) + l.epilogue()
	for _, f := range l.filters {
		if strings.Contains(msg, f) {
			return
		}
	}
	l.Print(msg)
}

func (l *minilogger) logln(level int, arg ...interface{}) {
	msg := l.prologue(level, "") + fmt.Sprint(arg...) + l.epilogue()
	for _, f := range l.filters {
		if strings.Contains(msg, f) {
			return
		}
	}
	l.Println(msg)
}

func Debug(format string, arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= DEBUG {
			logger.log(DEBUG, format, arg...)
		}
	}
}

func Info(format string, arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= INFO {
			logger.log(INFO, format, arg...)
		}
	}
}

func Warn(format string, arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= WARN {
			logger.log(WARN, format, arg...)
		}
	}
}

func Error(format string, arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= ERROR {
			logger.log(ERROR, format, arg...)
		}
	}
}

func Fatal(format string, arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= FATAL {
			logger.log(FATAL, format, arg...)
		}
	}
	os.Exit(1)
}

func Debugln(arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= DEBUG {
			logger.logln(DEBUG, arg...)
		}
	}
}

func Infoln(arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= INFO {
			logger.logln(INFO, arg...)
		}
	}
}

func Warnln(arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= WARN {
			logger.logln(WARN, arg...)
		}
	}
}

func Errorln(arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= ERROR {
			logger.logln(ERROR, arg...)
		}
	}
}

func Fatalln(arg ...interface{}) {
	logLock.Lock()
	defer logLock.Unlock()

	for _, logger := range loggers {
		if logger.Level <= FATAL {
			logger.logln(FATAL, arg...)
		}
	}
	os.Exit(1)
}
