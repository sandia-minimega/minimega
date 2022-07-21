// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func init() {
	// Setup up default logger to log to stdout at the debug level
	log.LevelFlag = log.DEBUG
	log.VerboseFlag = true

	log.Init()
	cliSetup()
}
