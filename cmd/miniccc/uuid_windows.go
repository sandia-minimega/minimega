// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build windows

package main

import (
	log "github.com/sandia-minimega/minimega/pkg/minilog"
	"os/exec"
	"regexp"
	"strings"
)

func getUUID() string {
	out, err := exec.Command("wmic", "path", "win32_computersystemproduct", "get", "uuid").CombinedOutput()
	if err != nil {
		log.Fatal("wmic run: %v", err)
	}

	// string must be in the form:
	//	XXXXXXXX-XXXX-XXXX-YYYY-YYYYYYYYYYYY
	re := regexp.MustCompile("[0-9a-z]{8}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{12}")

	uuid := re.FindString(strings.ToLower(string(out)))
	log.Debug("got UUID: %v", uuid)

	return uuid
}
