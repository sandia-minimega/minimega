// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func restartUpdated(path, update string) error {
	restore, err := installUpdate(path, update)
	if err != nil {
		return err
	}
	if err := syscall.Exec(path, os.Args, os.Environ()); err != nil {
		return errors.Join(fmt.Errorf("start updated client: %w", err), restore())
	}
	return nil
}
