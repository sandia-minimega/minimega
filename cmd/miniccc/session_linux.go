// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"slices"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func setSession(cmd *exec.Cmd, session ron.Session) (func(), error) {
	if !slices.Contains(ron.ValidSessions, session) {
		return nil, fmt.Errorf("unknown session: %v", session)
	}

	log.Debug("ignoring session %q: user sessions are not supported on Linux", session)
	return func() {}, nil
}

func lookPathEnv(file string, env []string) (string, error) {
	return exec.LookPath(file)
}
