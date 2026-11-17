package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// Tags with empty or multi-word values must survive the write/recover cycle.
func TestBaseConfigTagsRoundTrip(t *testing.T) {
	want := map[string]string{
		"empty": "",
		"desc":  "hello world",
		"plain": "x",
		"quote": `say "hi"  twice`,
	}

	var buf bytes.Buffer
	src := &BaseConfig{Tags: want}
	if err := src.WriteConfig(&buf); err != nil {
		t.Fatal(err)
	}

	dst := &BaseConfig{Tags: map[string]string{}}
	if err := dst.ReadConfig(strings.NewReader(buf.String()), "test-tags-roundtrip", nil); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(dst.Tags, want) {
		t.Errorf("got %q, want %q (written: %q)", dst.Tags, want, buf.String())
	}
}

// Configs written by older versions used unquoted values.
func TestBaseConfigTagsReadLegacy(t *testing.T) {
	dst := &BaseConfig{Tags: map[string]string{}}
	in := "vm config tags a b\nvm config tags c\nvm config tags note \"yes\"\nvm config tags n \"a\" b\n"
	if err := dst.ReadConfig(strings.NewReader(in), "test-tags-legacy", nil); err != nil {
		t.Fatal(err)
	}
	// legacy values are kept verbatim, quotes included
	want := map[string]string{"a": "b", "c": "", "note": `"yes"`, "n": `"a" b`}
	if !reflect.DeepEqual(dst.Tags, want) {
		t.Errorf("got %q, want %q", dst.Tags, want)
	}
}

// "vm config tags <key>" must print the value stored for key.
func TestVMConfigTagsLookupByKey(t *testing.T) {
	ns := &Namespace{Name: "test-vmconfig-tags-lookup"}
	addTestNamespace(t, ns)

	run := func(cmd string) *minicli.Response {
		t.Helper()

		c := minicli.MustCompile(cmd)
		c.SetNamespace(ns.Name)

		for _, h := range vmconfigerCLIHandlers {
			for _, p := range h.Patterns {
				if p != "vm config tags <key> [value]" {
					continue
				}
				ch := make(chan minicli.Responses, 1)
				h.Call(c, ch)
				return (<-ch)[0]
			}
		}
		t.Fatal("handler not found")
		return nil
	}

	run("vm config tags foo bar")
	if r := run("vm config tags foo"); r.Response != "bar" {
		t.Errorf("vm config tags foo = %q, want %q", r.Response, "bar")
	}
}
