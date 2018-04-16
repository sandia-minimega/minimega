// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"strconv"
	"strings"
	"time"
)

// parseDuration parses a duration, supporting a 'd' suffix in addition to those supported by time.ParseDuration.
// Returns the duration in minutes on success or -1 and error message on failure.
func parseDuration(s string) (int, error) {
	// duration is in minutes
	duration := 0

	v, err := strconv.Atoi(s)
	if err == nil {
		duration = v
	} else {
		index := strings.Index(s, "d")
		if index > 0 {
			days, err := strconv.Atoi(s[:index])
			if err != nil {
				return -1, err
			}
			duration = days * 24 * 60 // convert to minutes
		}

		if index+1 < len(s) {
			v, err := time.ParseDuration(s[index+1:])
			if err != nil {
				return -1, err
			}
			duration += int(v / time.Minute)
		}
	}

	return duration, nil

}
