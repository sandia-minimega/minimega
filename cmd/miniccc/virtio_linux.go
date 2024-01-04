// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux
// +build linux

package main

import (
	"io"
	"os"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func dialSerial(path string) (io.ReadWriteCloser, error) {
	log.Debug("ron dialSerial: %v", path)

	return os.OpenFile(path, os.O_RDWR, 0666)
}
