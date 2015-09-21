// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

func init() {
	// Setup up default logger to log to stdout at the debug level
	*f_log, *f_logfile, *f_loglevel = true, "", "debug"

	logSetup()
	cliSetup()
}
