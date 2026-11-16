// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// TestCLIPreprocessLeavesFileURLs checks that "file://" arguments, such as
// plumber file terminals, reach the command unchanged. Only the opaque form
// "file:name" refers to an iomeshage file.
func TestCLIPreprocessLeavesFileURLs(t *testing.T) {
	for _, v := range []string{
		"file:///tmp/minimega/files/out.txt",
		"file://relative/out.txt",
	} {
		got, err := cliPreprocess(v)
		if err != nil || got != v {
			t.Errorf("cliPreprocess(%q) = %q, %v; want it unchanged", v, got, err)
		}
	}
}

// TestCLIPreprocessorKeepsCCSendGlobs checks that cc send file globs reach the
// command without the "file:" prefix, so that cc send expands them on the mesh
// before it gets the files.
func TestCLIPreprocessorKeepsCCSendGlobs(t *testing.T) {
	base := useFileMesh(t)
	writeTestFiles(t, base, "one.txt")
	t.Setenv("CC_SEND_DIR", "dir")

	c := minicli.MustCompile("cc send file:*.txt *.log file:$CC_SEND_DIR/[ab].txt file:one.txt")
	if err := cliPreprocessor(c); err != nil {
		t.Fatal(err)
	}

	want := []string{"*.txt", "*.log", "dir/[ab].txt", filepath.Join(base, "one.txt")}
	if got := c.ListArgs["file"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %q, want %q", got, want)
	}
}
