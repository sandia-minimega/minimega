// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"fmt"
	log "minilog"
)

// CreateMirror creates a new tap that mirrors traffic from the bridge. Returns
// the created tap name or an error. Only one mirror can exist per bridge.
func (b *Bridge) CreateMirror() (string, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("creating mirror on bridge: %v", b.Name)

	if b.mirror != "" {
		return "", fmt.Errorf("bridge already has a mirror")
	}

	// get a host tap on VLAN 0
	tap, err := b.createTap("", 0, true)
	if err != nil {
		return "", err
	}

	// create the mirror for this bridge
	args := []string{
		"--",
		"--id=@p",
		"get",
		"port",
		tap,
		"--",
		"--id=@m",
		"create",
		"mirror",
		"name=m0",
		"select-all=true",
		"output-port=@p",
		"--",
		"set",
		"bridge",
		b.Name,
		"mirrors=@m",
	}

	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return "", fmt.Errorf("add mirror failed: %v: %v", err, sErr)
	}

	return tap, nil
}

// DestroyMirror destroys the previously created traffic mirror for the bridge,
// if one exists.
func (b *Bridge) DestroyMirror() error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.destroyMirror()
}

func (b *Bridge) destroyMirror() error {
	log.Info("destroying mirror on bridge: %v", b.Name)

	if b.mirror == "" {
		return fmt.Errorf("bridge does not have a mirror")
	}

	// delete the mirror for this bridge
	args := []string{
		"clear",
		"bridge",
		b.Name,
		"mirrors",
	}

	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("remove mirror failed: %v: %v", err, sErr)
	}

	// delete the associated host tap
	if err := b.destroyTap(b.mirror); err != nil {
		return err
	}

	b.mirror = ""
	return nil
}
