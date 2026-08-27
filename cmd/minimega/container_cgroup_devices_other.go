//go:build !linux

// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import "errors"

func containerAttachDeviceFilter(string, []string) error {
	return errors.New("cgroup device filtering requires Linux")
}
