// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"strconv"
	"strings"
	"time"
)

// parseDuration parses a duration, supporting a 'd' suffix in addition to
// those supported by time.ParseDuration. Rounds duration to minute.
func parseDuration(s string) (time.Duration, error) {
	// unitless integer is assumed to be in minutes
	if v, err := strconv.Atoi(s); err == nil {
		return time.Duration(v) * time.Minute, nil
	}

	var d time.Duration

	index := strings.Index(s, "d")
	if index > 0 {
		days, err := strconv.Atoi(s[:index])
		if err != nil {
			return -1, err
		}
		d = time.Duration(days*24) * time.Hour
	}

	if index+1 < len(s) {
		v, err := time.ParseDuration(s[index+1:])
		if err != nil {
			return -1, err
		}

		d += v
	}

	return d.Round(time.Minute), nil
}
