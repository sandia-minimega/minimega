// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"strconv"
	"strings"
	"sync"
)

const BlacklistedVLAN = "BLACKLISTED"
const VLANAliasSep = "//"
const VLANStart, VLANEnd = 2, 4096

type Range struct {
	min, max, next int
}

// AllocatedVLANs stores the state for the VLANs that we've allocated so far
type AllocatedVLANs struct {
	byVLAN  map[int]string
	byAlias map[string]int

	ranges map[string]*Range

	sync.Mutex
}

var allocatedVLANs = NewAllocatedVLANs()

func NewAllocatedVLANs() *AllocatedVLANs {
	return &AllocatedVLANs{
		byVLAN:  make(map[int]string),
		byAlias: make(map[string]int),
		ranges: map[string]*Range{
			"": &Range{
				min:  VLANStart,
				max:  VLANEnd,
				next: VLANStart,
			},
		},
	}
}

// broadcastUpdate sends out the updated VLAN mapping to all the nodes so that
// if the head node crashes we can recover which VLANs map to which aliases.
func (v *AllocatedVLANs) broadcastUpdate(alias string, vlan int) {
	cmd := minicli.MustCompilef("vlans add %v %v", alias, vlan)
	respChan := make(chan minicli.Responses)

	go func() {
		for resps := range respChan {
			for _, resp := range resps {
				if resp.Error != "" {
					log.Debug("unable to send alias %v -> %v to %v: %v", alias, vlan, resp.Host, resp.Error)
				}
			}
		}
	}()
	go meshageSend(cmd, Wildcard, respChan)
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

	return v.allocate(alias)
}

// allocate a VLAN for the alias. This should only be invoked if the caller has
// acquired the lock for v.
func (v *AllocatedVLANs) allocate(alias string) int {
	log.Debug("creating alias for %v", alias)

	// Find the next unallocated VLAN, taking into account that a range may be
	// specified for the supplied alias.
	r := v.ranges[""] // default
	for prefix, r2 := range v.ranges {
		if strings.HasPrefix(alias, prefix+VLANAliasSep) {
			r = r2
		}
	}

	// Find the next unallocated VLAN
outer:
	for {
		// Look to see if a VLAN is already allocated
		for v.byVLAN[r.next] != "" {
			r.next += 1
		}

		// Ensure that we're within the specified bounds
		if r.next >= r.max {
			// Ran out of VLANs... what is the right behavior?
			log.Fatal("ran out of VLANs")
		}

		// If we're in the default range, make sure we don't allocate anything
		// in a reserved range of VLANs
		if r == v.ranges[""] {
			for prefix, r2 := range v.ranges {
				if prefix == "" {
					continue
				}

				if r.next >= r2.min && r.next < r2.max {
					r.next = r2.max
					continue outer
				}
			}
		}

		// all the checks passed
		break
	}

	log.Debug("adding VLAN alias %v => %v", alias, r.next)

	v.byVLAN[r.next] = alias
	v.byAlias[alias] = r.next

	v.broadcastUpdate(alias, r.next)

	return r.next
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

// GetAliases returns a list of aliases with the given prefix.
func (v *AllocatedVLANs) GetAliases(prefix string) []string {
	v.Lock()
	defer v.Unlock()

	res := []string{}
	for k := range v.byAlias {
		if strings.HasPrefix(k, prefix) {
			res = append(res, k)
		}
	}

	return res
}

// Delete allocation for aliases matching a given prefix.
func (v *AllocatedVLANs) Delete(prefix string) {
	v.Lock()
	defer v.Unlock()

	log.Debug("deleting VLAN aliases with prefix: `%v`", prefix)

	for alias, vlan := range v.byAlias {
		if strings.HasPrefix(alias, prefix) {
			delete(v.byVLAN, vlan)
			delete(v.byAlias, alias)
		}
	}

	if prefix != "" {
		delete(v.ranges, strings.TrimSuffix(prefix, VLANAliasSep))
	}

	// Reset next counter so that we can find the recently freed VLANs
	for _, r := range v.ranges {
		r.next = r.min
	}
}

// SetRange reserves a range of VLANs for a particular prefix.
func (v *AllocatedVLANs) SetRange(prefix string, min, max int) error {
	v.Lock()
	defer v.Unlock()

	// Test for conflicts with other ranges
	for prefix2, r := range v.ranges {
		if prefix == prefix2 || prefix2 == "" {
			continue
		}

		if min <= r.max && r.min <= max {
			return fmt.Errorf("range overlaps with another namespace: %v", prefix2)
		}
	}

	// Warn if we detect any holes in the range
	for i := min; i <= max; i++ {
		if _, ok := v.byVLAN[i]; ok {
			log.Warn("detected hole in VLAN range %v -> %v: %v", min, max, i)
		}
	}

	v.ranges[prefix] = &Range{
		min:  min,
		max:  max,
		next: min,
	}

	return nil
}

// GetRanges returns a copy of the ranges currently in use.
func (v *AllocatedVLANs) GetRanges() map[string]Range {
	v.Lock()
	defer v.Unlock()

	res := map[string]Range{}
	for k := range v.ranges {
		// Create copy
		res[k] = *v.ranges[k]
	}

	return res
}

// Blacklist marks a VLAN as manually configured which removes it from the
// allocation pool. For instance, if a user runs `vm config net 100`, VLAN 100
// would be marked as blacklisted.
func (v *AllocatedVLANs) Blacklist(vlan int) {
	v.Lock()
	defer v.Unlock()

	v.blacklist(vlan)
}

// blacklist the VLAN. This should only be invoked if the caller has acquired
// the lock for v.
func (v *AllocatedVLANs) blacklist(vlan int) {
	if alias, ok := v.byVLAN[vlan]; ok {
		delete(v.byAlias, alias)
	}
	v.byVLAN[vlan] = BlacklistedVLAN
}

// GetBlacklist returns a list of VLANs that have been blacklisted.
func (v *AllocatedVLANs) GetBlacklist() []int {
	v.Lock()
	defer v.Unlock()

	res := []int{}
	for vlan, alias := range v.byVLAN {
		if alias == BlacklistedVLAN {
			res = append(res, vlan)
		}
	}

	return res
}

// ParseVLAN parses v and returns a VLAN. If v can be parsed as an integer, the
// resulting integer is returned. If v matches an existing alias, that VLAN is
// returned. Lastly, if none of the other cases are true and create is true, we
// will allocate a new alias for v, in the current namespace. Returns an error
// when create is false and v is not an integer or an alias.
func (v *AllocatedVLANs) ParseVLAN(s string, create bool) (int, error) {
	v.Lock()
	defer v.Unlock()

	vlan, err := strconv.Atoi(s)
	if err == nil {
		// Check to ensure that VLAN is sane
		if vlan < 0 || vlan >= 4096 {
			return 0, errors.New("invalid VLAN (0 <= vlan < 4096)")
		}

		if alias, ok := v.byVLAN[vlan]; ok && alias != BlacklistedVLAN {
			// Warn the user if they supplied an integer and it matches a VLAN
			// that has an alias.
			log.Warn("VLAN %d has alias %v", vlan, alias)
		} else if !ok {
			// Blacklist the VLAN if the user entered it manually and we don't
			// have an alias for it already.
			log.Warn("Blacklisting manually specified VLAN %v", vlan)
			allocatedVLANs.blacklist(vlan)
		}

		return vlan, nil
	}

	// Prepend active namespace if it doesn't look like the user is trying to
	// supply a namespace already.
	if !strings.Contains(s, VLANAliasSep) {
		s = namespace + VLANAliasSep + s
	}

	if vlan, ok := v.byAlias[s]; ok {
		return vlan, nil
	}

	if create {
		return v.allocate(s), nil
	}

	return 0, errors.New("unable to parse VLAN")
}

// PrintVLAN prints the alias for the VLAN, if one is set. Will trim off the
// namespace prefix if it matches the currently active namespace.
func (v *AllocatedVLANs) PrintVLAN(vlan int) string {
	v.Lock()
	defer v.Unlock()

	if alias, ok := v.byVLAN[vlan]; ok && alias != BlacklistedVLAN {
		// If we're in the namespace identified by the alias, we can trim off
		// the `<namespace>//` prefix.
		parts := strings.Split(alias, VLANAliasSep)
		if namespace == parts[0] {
			alias = strings.Join(parts[1:], VLANAliasSep)
		}

		return fmt.Sprintf("%v (%d)", alias, vlan)
	}

	return strconv.Itoa(vlan)
}
