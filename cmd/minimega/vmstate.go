// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
)

type VMState int

const (
	VM_BUILDING VMState = 1 << iota
	VM_RUNNING
	VM_PAUSED
	VM_QUIT
	VM_ERROR
)

// All VM states in one variable for masking any state
var VM_ANY_STATE VMState = ^0

// VM states that can be killed
var VM_KILLABLE = VM_BUILDING | VM_RUNNING | VM_PAUSED

func (s VMState) String() string {
	switch s {
	case VM_BUILDING:
		return "BUILDING"
	case VM_RUNNING:
		return "RUNNING"
	case VM_PAUSED:
		return "PAUSED"
	case VM_QUIT:
		return "QUIT"
	case VM_ERROR:
		return "ERROR"
	}
	return fmt.Sprintf("VmState(%d)", s)
}
