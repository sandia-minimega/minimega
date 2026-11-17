// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux
// +build linux

package main

import (
	"fmt"
	"io/ioutil"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var linuxUUIDPaths = []string{
	"/sys/devices/virtual/dmi/id/product_uuid",
}

func readLinuxUUID(readFile func(string) ([]byte, error)) (string, error) {
	var errors []string
	for _, path := range linuxUUIDPaths {
		d, err := readFile(path)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}

		uuid := strings.ToLower(strings.Trim(string(d), "\x00 \t\r\n"))
		if uuid == "" {
			errors = append(errors, fmt.Sprintf("%s: empty UUID", path))
			continue
		}

		return uuid, nil
	}

	return "", fmt.Errorf("no usable VM UUID source (%s)", strings.Join(errors, "; "))
}

// hostBoundChannel reports whether the host binds the connection to one VM
// and sets the UUID of that VM in the handshake.
func hostBoundChannel() bool {
	return *f_serial != "" || *f_family == "vsock"
}

// getUUID returns the UUID that miniccc claims when -uuid is not set.
func getUUID() string {
	return guestUUID(ioutil.ReadFile)
}

// guestUUID returns the DMI UUID of the guest, or an empty UUID on a
// host-bound channel. A UUID in guest memory stays the same when a snapshot
// is restored into another VM, but the host sets the UUID of the restored VM.
func guestUUID(readFile func(string) ([]byte, error)) string {
	if hostBoundChannel() {
		log.Debug("requesting host-bound identity")
		return ""
	}

	uuid, err := readLinuxUUID(readFile)
	if err != nil {
		log.Fatal("unable to get UUID: %v", err)
	}

	log.Debug("got UUID: %v", uuid)

	return uuid
}
