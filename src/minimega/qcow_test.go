// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"minicli"
	"testing"
)

var validInjectCommands = []string{
	`vm inject src src.qc2 "my file":"Program Files/my file"`,
	`vm inject dst dst.qc2 src src.qc2 "my file":"Program Files/my file"`,
	`vm inject src src.qc2 foo:"Program Files/my file"`,
	`vm inject src src.qc2 foo:"Program Files/my file" bar:dst/`,
	`vm inject src src.qc2 /bin/foo:dst/`,
}

func TestParseInject(t *testing.T) {
	for _, cmdStr := range validInjectCommands {
		cmd, err := minicli.CompileCommand(cmdStr)
		if err != nil {
			t.Fatalf("invalid command: %s", cmdStr)
		}

		inject := parseInject(cmd)
		if inject.err != nil {
			t.Errorf("unable to parse `%s`: %v", cmdStr, inject.err)
		}
	}
}
