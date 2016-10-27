// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build linux

package main

import (
	"io"
	log "minilog"
	"os"
)

func dialSerial(path string) (io.ReadWriteCloser, error) {
	log.Debug("ron dialSerial: %v", path)

	return os.OpenFile(path, os.O_RDWR, 0666)
}
