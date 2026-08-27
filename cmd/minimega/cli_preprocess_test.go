// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import "testing"

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
