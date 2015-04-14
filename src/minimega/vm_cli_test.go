// Copyright (2015) Sandia Corporation.
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

func TestVMConfig(t *testing.T) {
	for field := range vmConfigFns {
		t.Logf("testing vm config %s", field)

		switch field {
		case "memory", "vcpus":
			testVmConfigString(t, field)
		default:
			t.Logf("skipping vm config %s", field)
		}
	}
}

func TestKVMConfig(t *testing.T) {
	for field := range kvmConfigFns {
		t.Logf("testing kvm config %s", field)

		switch field {
		case "cdrom", "initrd", "kernel", "migrate", "uuid":
			testVmConfigString(t, field)
		case "snapshot":
			testVmConfigBool(t, field)
		case "disk", "qemu-append":
			testVmConfigStringSlice(t, field)
		default:
			t.Logf("skipping kvm config %s", field)
		}
	}
}

func testVmConfigString(t *testing.T, field string) {
	t.Logf("testing vm config %s", field)

	// Value we'll try to set
	want := "foo"

	// Compile getter, setter, and clear
	getCmd := mustCompile(t, "vm config %s", field)
	setCmd := mustCompile(t, "vm config %s %s", field, want)
	clrCmd := mustCompile(t, "clear vm config %s", field)

	testVmConfigField(t, want, getCmd, setCmd, clrCmd)
}

func testVmConfigBool(t *testing.T, field string) {
	values := []string{"true", "false"}

	for _, want := range values {
		t.Logf("testing vm config %s (%s)", field, want)

		// Compile getter, setter, and clear
		getCmd := mustCompile(t, "vm config %s", field)
		setCmd := mustCompile(t, "vm config %s %s", field, want)
		clrCmd := mustCompile(t, "clear vm config %s", field)

		testVmConfigField(t, want, getCmd, setCmd, clrCmd)
	}
}

func testVmConfigStringSlice(t *testing.T, field string) {
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

func TestVmConfigAppend(t *testing.T) {
	t.Logf("testing vm config append")

	// Value we'll try to set
	want := "ip=10.0.0.5 gateway=10.0.0.1 netmask=255.255.255.0 dns=10.10.10.10"

	// Compile getter, setter, and clear
	getCmd := mustCompile(t, "vm config append")
	setCmd := mustCompile(t, "vm config append %s", want)
	clrCmd := mustCompile(t, "clear vm config append")

	testVmConfigField(t, want, getCmd, setCmd, clrCmd)
}

func TestVmConfigQemuOverride(t *testing.T) {
	t.Logf("testing vm config net")

	overrides := [][]string{
		[]string{"randomstringthatmatchesnothing", "foo"},
		[]string{"qmp", "QED"},
		[]string{`" "`, `"  "`},
	}

	getCmd := mustCompile(t, "vm config qemu-override")
	clrCmd := mustCompile(t, "clear vm config qemu-override")

	for i := range overrides {
		orig := mustRun(t, getCmd)
		parts := strings.Split(orig, "\n\n")
		want := parts[2]

		// Add all the overrides up to the current
		for j := 0; j <= i; j++ {
			// Accumulate replacements in want
			want = strings.Replace(want, overrides[j][0], overrides[j][1], -1)

			addCmd := mustCompile(t, "vm config qemu-override add %s %s", overrides[j][0], overrides[j][1])
			mustRun(t, addCmd)

			parts = strings.Split(mustRun(t, getCmd), "\n\n")
			got := parts[2]

			if got != want {
				t.Errorf("got: `%s` != want: `%s`", got, want)
			}
		}

		// Clear all the overrides from this stage
		mustRun(t, clrCmd)

		got := mustRun(t, getCmd)
		if got != orig {
			t.Errorf("got: `%s` != orig: `%s`", got, orig)
		}
	}
}
