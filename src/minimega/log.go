package main

import (
	"errors"
	"fmt"
	log "minilog"
	"os"
)

func log_level(l string) (int, error) {
	switch l {
	case "debug":
		return log.DEBUG, nil
	case "info":
		return log.INFO, nil
	case "warn":
		return log.WARN, nil
	case "error":
		return log.ERROR, nil
	case "fatal":
		return log.FATAL, nil
	}
	return -1, errors.New("invalid log level")
}

func log_setup() {
	level, err := log_level(*f_loglevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *f_log {
		log.AddLogger("stdio", os.Stderr, level, true)
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

func cli_log_level(c cli_command) cli_response {
	if len(c.Args) == 0 {
		return cli_response{
			Response: *f_loglevel,
		}
	} else if len(c.Args) > 1 {
		return cli_response{
			Error: errors.New("log_level must be [debug, info, warn, error, fatal]"),
		}
	} else {
		level, err := log_level(c.Args[0])
		if err != nil {
			return cli_response{
				Error: err,
			}
		}
		*f_loglevel = c.Args[0]
		// forget the error, if they don't exist we shouldn't be setting their level, so we're fine.
		log.SetLevel("stdio", level)
		log.SetLevel("file", level)
	}
	return cli_response{}
}

func cli_log_stderr(c cli_command) cli_response {
	if len(c.Args) == 0 {
		_, err := log.GetLevel("stdio")
		if err != nil {
			return cli_response{
				Response: "false",
			}
		} else {
			return cli_response{
				Response: "true",
			}
		}
	} else if len(c.Args) > 1 {
		return cli_response{
			Error: errors.New("log_stderr takes only one argument"),
		}
	} else {
		_, err := log.GetLevel("stdio")
		switch c.Args[0] {
		case "true":
			level, _ := log_level(*f_loglevel)
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
			return cli_response{
				Error: errors.New("log_stderr must be [true, false]"),
			}
		}
	}
	return cli_response{}
}

func cli_log_file(c cli_command) cli_response {
	if len(c.Args) == 0 {
		_, err := log.GetLevel("file")
		if err != nil {
			return cli_response{
				Response: "false",
			}
		} else {
			return cli_response{
				Response: "true",
			}
		}
	} else if len(c.Args) > 1 {
		return cli_response{
			Error: errors.New("log_file takes only one argument"),
		}
	} else {
		_, err := log.GetLevel("file")

		// special case, disabling file logging with "false"
		if err == nil && c.Args[0] == "false" {
			log.DelLogger("file")
		} else {
			logfile, err := os.OpenFile(c.Args[0], os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
			if err != nil {
				return cli_response{
					Error: err,
				}
			}
			level, _ := log_level(*f_loglevel)
			log.AddLogger("file", logfile, level, false)
		}
	}
	return cli_response{}
}
