// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "minilog"
)

func init() {
	// Setup up default logger to log to stdout at the debug level
	*log.Verbose, *log.File, *log.Level = true, "", "debug"

	log.Init()
	cliSetup()
}
