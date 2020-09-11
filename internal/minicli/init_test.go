// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli_test

import (
	log "github.com/sandia-minimega/minimega/pkg/minilog"
)

func init() {
	// Setup up default logger to log to stdout at the debug level
	log.LevelFlag = log.DEBUG
	log.VerboseFlag = true

	log.Init()
}
