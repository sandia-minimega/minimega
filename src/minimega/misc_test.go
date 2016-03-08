// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"minicli"
	"testing"
)

func TestHasCommand(t *testing.T) {
	// Make some dummy commands with nested subcommands
	strs := []string{"foo", "bar", "baz"}
	cmds := []*minicli.Command{}
	for i, str := range strs {
		var sub *minicli.Command
		if i != 0 {
			sub = cmds[i-1]
		}

		cmds = append(cmds, &minicli.Command{Original: str, Subcommand: sub})
	}

	for i := range cmds {
		// Test where we know should have command
		for j := len(cmds) - 1; j >= i; j-- {
			if !hasCommand(cmds[j], strs[i]) {
				t.Errorf("expected cmd %d to have `%v`", j, strs[i])
			}
		}

		// Test where we know we should *not* have command
		for j := 0; j < i; j++ {
			if hasCommand(cmds[j], strs[i]) {
				t.Errorf("expected cmd %d not to have `%v`", j, strs[i])
			}
		}
	}
}

func TestQuotedJoin(t *testing.T) {
	testData := []struct {
		v    []string
		want string
	}{
		{[]string{"a", "b"}, `a b`},
		{[]string{"a b", "c"}, `"a b" c`},
		{[]string{"a\tb", "c\nd"}, `"a\tb" "c\nd"`},
	}

	for _, d := range testData {
		got := quotedJoin(d.v)
		if got != d.want {
			t.Errorf("got: `%v` != want: `%v`", got, d.want)
		}
	}
}
