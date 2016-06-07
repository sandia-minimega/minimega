// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"fmt"
	"ipmac"
	log "minilog"
)

// CreateTap creates and adds a tap to a bridge. If a name is not provided, one
// will be automatically generated.
func (b *Bridge) CreateTap(tap string, lan int, host bool) (string, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.createTap(tap, lan, host)
}

func (b *Bridge) createTap(t string, lan int, host bool) (tap string, err error) {
	log.Info("creating tap on bridge: %v %v", b.Name, t)

	// reap taps before creating to avoid someone killing/restarting a vm
	// faster than the periodic tap reaper
	b.reapTaps()

	if _, ok := b.taps[t]; ok {
		return t, fmt.Errorf("tap already on bridge")
	}

	tap = t
	if tap == "" {
		tap = <-b.nameChan
	}

	var existed bool

	// TODO: does this make sense? shouldn't create fail if the tap already exists?
	if err := createTap(tap); err == errAlreadyExists && t != "" {
		// Caller provided a name so assume it was created for us
		existed = true
	} else if err != nil {
		return "", err
	}

	// Clean up the tap we just created, if it didn't already exist.
	defer func() {
		if err != nil && !existed {
			if err := destroyTap(tap); err != nil {
				// Welp, we're boned
				log.Error("zombie tap -- %v %v", tap, err)
			}
			tap = ""
		}
	}()

	if err := upInterface(tap, host); err != nil {
		return "", err
	}

	return tap, b.addTap(tap, lan, host)
}

// AddTap adds an existing tap to the bridge. Can be used in conjunction with
// `Bridge.RemoveTap` to relocate tap to a different bridge or VLAN.
func (b *Bridge) AddTap(tap string, lan int, host bool) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.addTap(tap, lan, host)
}

func (b *Bridge) addTap(tap string, lan int, host bool) error {
	log.Info("adding tap on bridge: %v %v", b.Name, tap)

	// reap taps before adding to avoid someone killing/restarting a vm faster
	// than the periodic tap reaper
	b.reapTaps()

	if _, ok := b.taps[tap]; ok {
		return fmt.Errorf("tap already on bridge")
	}

	err := ovsAddPort(b.Name, tap, lan, host)
	if err == errAlreadyExists {
		// Special case -- tap is already on bridge... try to remove it first
		// and then add it again.
		log.Info("tap %v is already on bridge, adding again", tap)
		if err = ovsDelPort(b.Name, tap); err == nil {
			err = ovsAddPort(b.Name, tap, lan, host)
		}
	}

	if err != nil {
		return err
	}

	b.taps[tap] = &Tap{
		Name:   tap,
		Bridge: b.Name,
		VLAN:   lan,
		Host:   host,
	}

	return nil
}

// DestroyTap removes a tap from the bridge and marks it as defunct. See
// `Bridge.ReapTaps` to clean up defunct taps.
func (b *Bridge) DestroyTap(tap string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.destroyTap(tap)
}

// destroyTap cleans up the underlying device, ReapTaps will actually remove
// the tap from the bridge and list of taps on the bridge.
func (b *Bridge) destroyTap(t string) error {
	log.Info("destroying tap on bridge: %v %v", b.Name, t)

	tap, ok := b.taps[t]
	if !ok {
		return fmt.Errorf("unknown tap: %v", t)
	}

	tap.Defunct = true

	b.clearQos(tap)

	if tap.Container {
		return destroyVeth(tap.Name)
	}

	return destroyTap(tap.Name)
}

// RemoveTap removes a tap from the bridge but doesn't remove the underlying
// device so that it may be added to another bridge. See `Bridge.AddTap`.
func (b *Bridge) RemoveTap(tap string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("removing tap from bridge: %v %v", b.Name, tap)

	if err := ovsDelPort(b.Name, tap); err != nil {
		return err
	}

	delete(b.taps, tap)
	return nil
}

// startIML starts the MAC listener for this bridge.
func (b *Bridge) startIML() {
	// use openflow to redirect arp and icmp6 traffic to the local tap
	filters := []string{
		"dl_type=0x0806,actions=local,normal",
		"dl_type=0x86dd,nw_proto=58,icmp_type=135,actions=local,normal",
	}

	for _, filter := range filters {
		if err := b.addOpenflow(filter); err != nil {
			log.Error("cannot start ip learner on bridge: %v", err)
			return
		}
	}

	iml, err := ipmac.NewLearner(b.Name)
	if err != nil {
		log.Error("cannot start ip learner on bridge: %v", err)
		return
	}

	b.IPMacLearner = iml
}

// addOpenflow adds an openflow rule to the bridge using `ovs-ofctl`.
func (b *Bridge) addOpenflow(filter string) error {
	out, err := processWrapper("ovs-ofctl", "add-flow", b.Name, filter)
	if err != nil {
		return fmt.Errorf("add openflow failed: %v: %v", err, out)
	}

	return nil
}
