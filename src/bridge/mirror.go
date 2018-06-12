// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"errors"
	"fmt"
	log "minilog"
)

// CreateMirror mirrors traffic. src is the tap to mirror, an empty src implies
// mirroring the entire bridge. dst is the tap to mirror to, an empty dst will
// use an automatically generated tap name. Returns the dst tap name or an
// error.
func (b *Bridge) CreateMirror(src, dst string) (string, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	// check src tap exists *before* we go ahead and assign a name
	if _, ok := b.taps[src]; src != "" && !ok {
		return "", errors.New("source tap is not on bridge")
	}

	if dst == "" {
		dst = <-b.nameChan
	}

	return b.createMirror(src, dst)
}

func (b *Bridge) createMirror(src, dst string) (string, error) {
	log.Info("adding mirror on bridge: %v:%v to %v", b.Name, src, dst)

	// get a host tap on VLAN 0
	if err := b.createHostTap(dst, 0); err != nil {
		return "", err
	}

	if err := ovsMirror(b.Name, src, dst); err != nil {
		// Clean up the tap we just created
		if err := b.destroyTap(dst); err != nil {
			// Welp, we're boned
			log.Error("zombie tap -- %v %v", dst, err)
		}

		return "", fmt.Errorf("add mirror failed: %v", err)
	}

	b.mirrors[dst] = true

	return dst, nil
}

func (b *Bridge) DestroyMirror(tap string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.destroyMirror(tap)
}

func (b *Bridge) destroyMirror(tap string) error {
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
