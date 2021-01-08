// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"
	log "minilog"
	"strings"
)

func (b *Bridge) Config(s string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return errors.New("expected key=value")
	}

	log.Info("setting bridge config on %v: %v", b.Name, s)

	args := []string{
		"set",
		"bridge",
		b.Name,
		s,
	}
	if _, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("set config failed: %v", err)
	}

	b.config[parts[0]] = parts[1]
	return nil
}
