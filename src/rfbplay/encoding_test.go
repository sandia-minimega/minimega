// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"testing"
)

func TestDecodeCLength(t *testing.T) {
	buf := bytes.NewReader([]byte{0x90, 0x4e})

	res, err := DecodeCLength(buf)
	if err != nil {
		t.Fatal(err)
	}

	if res != 10000 {
		t.Error("wrong value,", res)
	}
}
