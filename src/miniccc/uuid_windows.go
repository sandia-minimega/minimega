// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build windows

package main

import (
	log "minilog"
	"os/exec"
	"strings"
)

func getUUID() string {
	out, err := exec.Command("wmic", "path", "win32_computersystemproduct", "get", "uuid").CombinedOutput()
	if err != nil {
		log.Fatal("wmic run: %v", err)
	}

	uuid := strings.TrimSpace(string(out))
	log.Debug("got UUID: %v", uuid)

	return uuid
}
