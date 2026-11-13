// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

func TestRecvPartsReadsAllPartsAfterWriteError(t *testing.T) {
	// A destination below a regular file cannot be created.
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, nil, 0600); err != nil {
		t.Fatal(err)
	}

	parts := make(chan *ron.Message, 3)
	for i := 0; i < 3; i++ {
		parts <- &ron.Message{
			Type: ron.MESSAGE_FILE,
			File: &ron.File{
				Name:   "foo",
				Data:   []byte("x"),
				Offset: int64(i),
				EOF:    i == 2,
			},
		}
	}

	if got := recvParts(parts, "foo", filepath.Join(parent, "foo")); got != 0 {
		t.Fatalf("bytes written = %d, want 0", got)
	}

	if n := len(parts); n != 0 {
		t.Fatalf("parts left after the transfer = %d, want 0", n)
	}
}

func TestRecvDir(t *testing.T) {
	base := filepath.FromSlash("/tmp/miniccc")

	cases := map[string]string{
		"":         filepath.Join(base, "files"),
		"sub/dir":  filepath.Join(base, "files", "sub", "dir"),
		"/opt/foo": filepath.FromSlash("/opt/foo"),
	}

	for destination, want := range cases {
		if got := recvDir(base, destination); got != want {
			t.Errorf("recvDir(%q) = %q, want %q", destination, got, want)
		}
	}
}
