// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// AddTrunk add an existing interface as a trunk port to the bridge.
func (b *Bridge) AddTrunk(iface string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("adding trunk port on bridge %v: %v", b.Name, iface)

	if b.trunks[iface] {
		return fmt.Errorf("bridge already trunking to %v", iface)
	}

	err := ovsAddPort(b.Name, iface, 0, false)
	if err == nil {
		b.trunks[iface] = true
	}

	return err
}

// RemoveTrunk removes a trunk port from the bridge.
func (b *Bridge) RemoveTrunk(iface string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.removeTrunk(iface)
}

func (b *Bridge) removeTrunk(iface string) error {
	log.Info("removing trunk from bridge %v: %v", b.Name, iface)

	if !b.trunks[iface] {
		return fmt.Errorf("unknown trunk: %v", iface)
	}

	err := ovsDelPort(b.Name, iface)
	if err == nil {
		delete(b.trunks, iface)
	}

	return err
}
