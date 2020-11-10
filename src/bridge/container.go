// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"fmt"
	log "minilog"
)

// CreateContainerTap creates a veth tap and adds it to the bridge. tap is the
// name of the tap, it will be automatically generated if unspecified. ns is
// the network namespace for the tap. mac is the MAC address to assign to the
// interface. vlan is the VLAN for the traffic. index is the veth interface
// number for the container.
func (b *Bridge) CreateContainerTap(tap, ns, mac string, vlan, index int) (string, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("creating veth tap on bridge %v: %v %v %v %v %v", b.Name, tap, ns, mac, vlan, index)

	// reap taps before creating to avoid someone killing/restarting a vm
	// faster than the periodic tap reaper
	b.reapTaps()

	if _, ok := b.taps[tap]; ok {
		return tap, fmt.Errorf("tap already on bridge")
	}

	if tap == "" {
		tap = <-b.nameChan
	}

	// name of the interface inside the container
	iface := fmt.Sprintf("veth%v", index)

	var created bool

	err := createVeth(tap, iface, ns)
	if err == nil {
		created = true
		err = upInterface(tap, false)
	}
	if err == nil {
		err = setMAC(ns, iface, mac)
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
