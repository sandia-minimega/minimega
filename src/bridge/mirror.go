// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"fmt"
	log "minilog"
)

// addMirror creates a new tap that mirrors traffic from the bridge. Returns
// the created tap name or an error.
func (b *Bridge) addMirror() (string, error) {
	log.Info("adding mirror on bridge: %v", b.Name)

	// get a host tap on VLAN 0
	tap := <-b.nameChan
	if err := b.createHostTap(tap, 0); err != nil {
		return "", err
	}

	// create the mirror for this bridge
	args := []string{
		// get the tap ID, store in @p
		"--",
		"--id=@p",
		"get",
		"port",
		tap,

		// create a new mirror whose ID is @m, mirror to @p
		"--",
		"--id=@m",
		"create",
		"mirror",
		fmt.Sprintf("name=mirror-%v", tap),
		"select-all=true",
		"output-port=@p",

		// add mirror to bridge
		"--",
		"add",
		"bridge",
		b.Name,
		"mirrors",
		"@m",
	}

	if _, err := ovsCmdWrapper(args); err != nil {
		// Clean up the tap we just created
		if err := b.destroyTap(tap); err != nil {
			// Welp, we're boned
			log.Error("zombie tap -- %v %v", tap, err)
		}

		return "", fmt.Errorf("add mirror failed: %v", err)
	}

	b.mirrors[tap] = true

	return tap, nil
}

func (b *Bridge) removeMirror(tap string) error {
	log.Info("removing mirror on bridge %v: %v", b.Name, tap)

	if !b.mirrors[tap] {
		return fmt.Errorf("tap is not a mirror on bridge %v: %v", b.Name, tap)
	}

	// delete the mirror for this bridge
	args := []string{
		// get mirror ID by name, store in @m
		"--",
		"--id=@m",
		"get",
		"mirror",
		fmt.Sprintf("mirror-%v", tap),

		// remove mirror from bridge
		"--",
		"remove",
		"bridge",
		b.Name,
		"mirrors",
		"@m",
	}

	if _, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("remove mirror failed: %v", err)
	}

	// delete the associated host tap
	if err := b.destroyTap(tap); err != nil {
		return err
	}

	delete(b.mirrors, tap)
	return nil
}
