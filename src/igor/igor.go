// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.
package main

import (
	"os/user"
	"time"
)

// igor holds globals
var igor struct {
	Config       // embed
	Reservations // embed
	Backend      // embed
	*user.User   // embed

	// Now is the time when igor started, used for a consistent view of "now"
	// across functions
	Now time.Time
}
