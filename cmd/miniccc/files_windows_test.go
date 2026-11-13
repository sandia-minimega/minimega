// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import "testing"

func TestRecvDirWindows(t *testing.T) {
	cases := map[string]string{
		`\opt\foo`:            `\opt\foo`,
		`C:/`:                 `C:\`,
		`//server/share/drop`: `\\server\share\drop`,
	}

	for destination, want := range cases {
		if got := recvDir(`C:\miniccc`, destination); got != want {
			t.Errorf("recvDir(%q) = %q, want %q", destination, got, want)
		}
	}
}
