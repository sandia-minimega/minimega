// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// restartUpdated starts the updated client as a new process and exits. Windows
// lets a process rename its own running executable, but not overwrite it. A
// service exits instead, and its recovery action starts the new binary.
func restartUpdated(path, update string) error {
	if runAsService {
		return exitForRestart(path, update, os.Exit)
	}

	restore, err := installUpdate(path, update)
	if err != nil {
		return err
	}

	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return errors.Join(fmt.Errorf("start updated client: %w", err), restore())
	}
	os.Exit(0)
	return nil
}
