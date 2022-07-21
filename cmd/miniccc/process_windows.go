// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build windows
// +build windows

package main

func processExists(pid int) bool {
	// doesn't matter, not used
	return false
}
