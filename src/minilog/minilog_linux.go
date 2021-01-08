// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

// +build linux

package minilog

import (
	"log/syslog"
)

// Helper function to add syslog output by connecting to address raddr on the
// specified network. Events are logged with a specified tag. Calling more than
// once overwrites existing syslog writers. If network == "local", log to the
// local syslog daemon.
func AddSyslog(network, raddr, tag string, level Level) error {
	var w *syslog.Writer
	var err error

	priority := syslog.LOG_INFO | syslog.LOG_DAEMON

	if network == "local" {
		w, err = syslog.New(priority, tag)
	} else {
		w, err = syslog.Dial(network, raddr, priority, tag)
	}
	if err != nil {
		return err
	}

	AddLogger("syslog", w, level, false)
	return nil
}
