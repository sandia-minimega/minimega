// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	log "minilog"
	"strings"
)

func getCobblerProfiles() map[string]bool {
	res := map[string]bool{}

	// Get a list of current profiles
	out, err := processWrapper("cobbler", "profile", "list")
	if err != nil {
		log.Fatal("couldn't get list of cobbler profiles: %v\n", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		res[strings.TrimSpace(scanner.Text())] = true
	}

	if err := scanner.Err(); err != nil {
		log.Fatal("unable to read cobbler profiles: %v", err)
	}

	return res
}
