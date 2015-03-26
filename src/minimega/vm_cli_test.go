// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	"strings"
	"testing"
)

func mustRun(t *testing.T, cmd *minicli.Command) string {
	var err error
	var res string

	t.Logf("running command `%v`", cmd.Original)

	for resps := range runCommand(cmd, false) {
		for _, resp := range resps {
			if resp.Error != "" {
				err = errors.New(resp.Error)
			} else if res != "" {
				err = errors.New("got more than one response from command")
			}

			res = resp.Response
		}
	}

	if err != nil {
		t.Fatalf("error running command `%v` -- %v", cmd.Original, err)
	}

	return res
}

func mustCompile(t *testing.T, format string, args ...interface{}) *minicli.Command {
	str := fmt.Sprintf(format, args...)
	t.Logf("compiling command `%v`", str)

	cmd, err := minicli.CompileCommand(str)
	if err != nil {
		t.Fatal("unable to compile `%v` -- %v", str, err)
	}

	return cmd
}

func testVmConfigField(t *testing.T, want string,
	getCmd, setCmd, clrCmd *minicli.Command) {

	// Determine the default value
	orig := mustRun(t, getCmd)

	// Set the value and test that it was set correctly
	mustRun(t, setCmd)
	got := mustRun(t, getCmd)
	if got != want {
		t.Errorf("got: `%s` != want: `%s`", got, want)
	}

	// Now, try clearing it and see if we get the orig value back
	mustRun(t, clrCmd)
	got = mustRun(t, getCmd)
	if got != orig {
		t.Errorf("got: `%s` != orig: `%s`", got, orig)
	}
}

func TestVmConfigStrings(t *testing.T) {
	for _, field := range vmInfoStringFields {
		t.Logf("testing vm config %s", field)

		// Value we'll try to set
		want := "foo"

		// Compile getter, setter, and clear
		getCmd := mustCompile(t, "vm config %s", field)
		setCmd := mustCompile(t, "vm config %s %s", field, want)
		clrCmd := mustCompile(t, "clear vm config %s", field)

		testVmConfigField(t, want, getCmd, setCmd, clrCmd)
	}
}

func TestVmConfigBools(t *testing.T) {
	values := []string{"true", "false"}

	for _, field := range vmInfoBoolFields {
		for _, want := range values {
			t.Logf("testing vm config %s (%s)", field, want)

			// Compile getter, setter, and clear
			getCmd := mustCompile(t, "vm config %s", field)
			setCmd := mustCompile(t, "vm config %s %s", field, want)
			clrCmd := mustCompile(t, "clear vm config %s", field)

			testVmConfigField(t, want, getCmd, setCmd, clrCmd)
		}
	}
}

func TestVmConfigStringSlices(t *testing.T) {
	for _, field := range vmInfoStringSliceFields {
		t.Logf("testing vm config %s", field)

		// Value we'll try to set
		values := []string{"foo", "bar", "baz"}
		want := fmt.Sprintf("%v", values)

		// Compile getter, setter, and clear
		getCmd := mustCompile(t, "vm config %s", field)
		setCmd := mustCompile(t, "vm config %s %s", field, strings.Join(values, " "))
		clrCmd := mustCompile(t, "clear vm config %s", field)

		testVmConfigField(t, want, getCmd, setCmd, clrCmd)
	}
}

// TODO: Test append, net, qemu-override
