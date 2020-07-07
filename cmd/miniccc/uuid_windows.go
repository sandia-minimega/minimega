// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build windows
// +build windows

package main

import (
	"os/exec"
	"regexp"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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
