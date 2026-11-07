// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// CreateMirror mirrors traffic. src is the tap to mirror, an empty src implies
// mirroring the entire bridge. dst is the tap to mirror to, which must already
// exist. namespace owns the mirror.
func (b *Bridge) CreateMirror(src, dst, namespace string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.createMirror(src, dst, MirrorTags{Namespace: namespace, Kind: MirrorKindTap})
}

func (b *Bridge) createMirror(src, dst string, tags MirrorTags) error {
	log.Info("adding mirror on bridge: %v:%v to %v", b.Name, src, dst)
	if src == dst {
		return errors.New("cannot mirror tap to itself")
	}

	// check source tap exists, if it is set
	if _, ok := b.taps[src]; src != "" && !ok {
		return errors.New("source tap is not on bridge")
	}

	// check destination tap exists
	if _, ok := b.taps[dst]; !ok {
		return errors.New("destination tap is not on bridge")
	}

	// make sure we aren't mirroring to the same tap twice
	if b.mirrors[dst] {
		return errors.New("destination tap is already a mirror")
	}

	if err := checkMirrorBudget(b.Name); err != nil {
		return err
	}

	if err := ovsAddMirror(b.Name, src, dst, tags); err != nil {
		return err
	}

	b.mirrors[dst] = true

	return nil
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

	if err := ovsDelMirror(b.Name, tap); err != nil {
		return err
	}

	delete(b.mirrors, tap)
	return nil
}
