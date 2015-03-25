// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build linux

package ron

import (
	"io/ioutil"
	log "minilog"
	"strings"
)

func getUUID() (string, error) {
	d, err := ioutil.ReadFile("/sys/devices/virtual/dmi/id/product_uuid")
	if err != nil {
		return "", err
	}
	uuid := string(d[:len(d)-1])
	uuid = strings.ToLower(uuid)
	log.Debug("got UUID: %v", uuid)
	return uuid, nil
}
