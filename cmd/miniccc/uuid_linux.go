// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux
// +build linux

package main

import (
	"io/ioutil"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func getUUID() string {
	d, err := ioutil.ReadFile("/sys/devices/virtual/dmi/id/product_uuid")
	if err != nil {
		log.Fatal("unable to get UUID: %v", err)
	}

	uuid := strings.ToLower(strings.TrimSpace(string(d)))
	log.Debug("got UUID: %v", uuid)

	return uuid
}
