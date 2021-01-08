// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

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
