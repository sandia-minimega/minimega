// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	log "minilog"
	"strings"
	"sync"
)

const BlacklistedVLAN = "BLACKLISTED"
const VLANAliasSep = "//"
const VLANStart = 2

// AllocatedVLANs stores the state for the VLANs that we've allocated so far
type AllocatedVLANs struct {
	byVLAN  map[int]string
	byAlias map[string]int

	next int

	sync.Mutex
}

var allocatedVLANs = AllocatedVLANs{
	byVLAN:  make(map[int]string),
	byAlias: make(map[string]int),
	next:    VLANStart,
}

// GetOrAllocate looks up the VLAN for the provided alias. If one has not
// already been assigned, it will allocate the next available VLAN.
func (v *AllocatedVLANs) GetOrAllocate(alias string) int {
	if vlan, ok := v.byAlias[alias]; ok {
		return vlan
	}

	// Not assigned, find the next VLAN
	v.Lock()
	defer v.Unlock()

	// Find the next unallocated VLAN
	for v.byVLAN[v.next] != "" {
		v.next += 1
	}

	if v.next > 4095 {
		// Ran out of VLANs... what is the right behavior?
		log.Fatal("ran out of VLANs")
	}

	log.Debug("adding VLAN alias %v => %v", alias, v.next)

	v.byVLAN[v.next] = alias
	v.byAlias[alias] = v.next

	return v.next
}

// AddAlias sets the VLAN for the provided alias.
func (v *AllocatedVLANs) AddAlias(alias string, vlan int) error {
	v.Lock()
	defer v.Unlock()

	log.Debug("adding VLAN alias %v => %v", alias, vlan)

	if _, ok := v.byAlias[alias]; ok {
		return errors.New("alias already in use")
	}
	if _, ok := v.byVLAN[vlan]; ok {
		return errors.New("vlan already in use")
	}

	v.byVLAN[vlan] = alias
	v.byAlias[alias] = vlan

	return nil
}

// GetVLAN returns the alias for a given VLAN or DisconnectedVLAN if it has not
// been assigned an alias.
func (v *AllocatedVLANs) GetVLAN(alias string) int {
	v.Lock()
	defer v.Unlock()

	if vlan, ok := v.byAlias[alias]; ok {
		return vlan
	}

	return DisconnectedVLAN
}

// GetAlias returns the alias for a given VLAN or the empty string if it has
// not been assigned an alias. Note that previously Blacklist'ed VLANs will
// return the const BlacklistedVLAN.
func (v *AllocatedVLANs) GetAlias(vlan int) string {
	v.Lock()
	defer v.Unlock()

	return v.byVLAN[vlan]
}

// Delete allocation for aliases matching a given prefix.
func (v *AllocatedVLANs) Delete(prefix string) {
	v.Lock()
	defer v.Unlock()

	for alias, vlan := range v.byAlias {
		if strings.HasPrefix(alias, prefix) {
			delete(v.byVLAN, vlan)
			delete(v.byAlias, alias)
		}
	}

	// Reset next counter so that we can find the recently freed VLANs
	v.next = VLANStart
}

// Blacklist marks a VLAN as manually configured which removes it from the
// allocation pool. For instance, if a user runs `vm config net 100`, VLAN 100
// would be marked as blacklisted.
//
// TODO: Currently there is no way to free the Blacklist'ed VLAN, even when
// calling `clear vlans`. Should we be able to free them?
func (v *AllocatedVLANs) Blacklist(vlan int) {
	v.Lock()
	defer v.Unlock()

	if alias, ok := v.byVLAN[vlan]; ok {
		delete(v.byAlias, alias)
	}
	v.byVLAN[vlan] = BlacklistedVLAN
}
