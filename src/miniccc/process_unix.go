// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build linux

package main

import (
	"os"
	"path/filepath"
	"strconv"
)

func processExists(pid int) bool {
	fname := filepath.Join("/proc", strconv.Itoa(pid))
	_, err := os.Stat(fname)
	return os.IsExist(err)
}
