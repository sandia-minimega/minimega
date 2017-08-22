// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"fmt"
	log "minilog"
)

// CreateTap creates a new tap and adds it to the bridge. mac is the MAC
// address to assign to the interface. vlan is the VLAN for the traffic.
func (b *Bridge) CreateTap(mac string, vlan int) (string, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("creating tap on bridge %v: %v %v", b.Name, mac, vlan)

	// reap taps before creating to avoid someone killing/restarting a vm
	// faster than the periodic tap reaper
	b.reapTaps()

	tap := <-b.nameChan

	var created bool

	err := createTap(tap)
	if err == nil {
		created = true
		err = upInterface(tap, false)
	}
	if err == nil {
		err = b.addTap(tap, mac, vlan, false)
	}

	// clean up the tap we created
	if err != nil && created {
		if err := destroyTap(tap); err != nil {
			// Welp, we're boned
			log.Error("zombie tap -- %v %v", tap, err)
		}

		return "", err
	}

	return tap, nil
}

// CreateHostTap creates and adds a host tap to a bridge. If a name is not
// provided, one will be automatically generated.
func (b *Bridge) CreateHostTap(tap string, lan int) (string, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.createHostTap(tap, lan)
}

func (b *Bridge) createHostTap(t string, lan int) (string, error) {
	log.Info("creating host tap on bridge: %v %v", b.Name, t)

	// reap taps before creating to avoid someone killing/restarting a vm
	// faster than the periodic tap reaper
	b.reapTaps()

	if _, ok := b.taps[t]; ok {
		return t, fmt.Errorf("tap already on bridge")
	}

	tap := t
	if tap == "" {
		tap = <-b.nameChan
	}

	if err := b.addTap(tap, "", lan, true); err != nil {
		return "", err
	}

	if err := upInterface(tap, true); err != nil {
		// Clean up the tap we just created
		if err := b.destroyTap(tap); err != nil {
			// Welp, we're boned
			log.Error("zombie tap -- %v %v", tap, err)
		}

		return "", err
	}

	return tap, nil
}

// AddTap adds an existing tap to the bridge. Can be used in conjunction with
// `Bridge.RemoveTap` to relocate tap to a different bridge or VLAN.
func (b *Bridge) AddTap(tap, mac string, lan int, host bool) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.addTap(tap, mac, lan, host)
}

func (b *Bridge) addTap(tap, mac string, lan int, host bool) error {
	log.Info("adding tap on bridge: %v %v %v %v %v", b.Name, tap, mac, lan, host)

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
		MAC:    mac,
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

	if tap.Host {
		// Tap is managed by OVS -- calling del-port will delete it for us.
		return nil
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

// addOpenflow adds an openflow rule to the bridge using `ovs-ofctl`.
func (b *Bridge) addOpenflow(filter string) error {
	out, err := processWrapper("ovs-ofctl", "add-flow", b.Name, filter)
	if err != nil {
		return fmt.Errorf("add openflow failed: %v: %v", err, out)
	}

	return nil
}
