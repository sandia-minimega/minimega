// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"minicli"
	"testing"
)

var validInjectCommands = []string{
	`disk inject image.qc2 "my file":"Program Files/my file"`,
	`disk inject image.qc2 foo:"Program Files/my file"`,
	`disk inject image.qc2 foo:"Program Files/my file" bar:dst/`,
	`disk inject image.qc2 /bin/foo:dst/`,
}

func TestParseInject(t *testing.T) {
	for _, cmdStr := range validInjectCommands {
		_, err := minicli.Compile(cmdStr)
		if err != nil {
			t.Fatalf("invalid command: %s", cmdStr)
		}
	}
}
