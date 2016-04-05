// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package vlans

import (
	"errors"
	"fmt"
	log "minilog"
	"strconv"
	"strings"
	"sync"
)

const BlacklistedVLAN = "BLACKLISTED"
const AliasSep = "//"
const VLANStart, VLANEnd = 101, 4096

var ErrUnknownVLAN = errors.New("unknown VLAN")
var ErrUnknownAlias = errors.New("unknown alias")
var ErrOutOfVLANs = errors.New("out of VLANs")

type Range struct {
	Min, Max, Next int
}

// AllocatedVLANs stores the state for the VLANs that we've allocated so far
type AllocatedVLANs struct {
	byVLAN  map[int]string
	byAlias map[string]int

	ranges map[string]*Range

	sync.Mutex
}

func NewAllocatedVLANs() *AllocatedVLANs {
	return &AllocatedVLANs{
		byVLAN:  make(map[int]string),
		byAlias: make(map[string]int),
		ranges: map[string]*Range{
			"": &Range{
				Min:  VLANStart,
				Max:  VLANEnd,
				Next: VLANStart,
			},
		},
	}
}

// Allocate looks up the VLAN for the provided alias. If one has not already
// been assigned, it will allocate the next available VLAN. Returns the VLAN
// and flag for whether the alias was created or not.
func (v *AllocatedVLANs) Allocate(namespace, s string) (int, bool, error) {
	v.Lock()
	defer v.Unlock()

	// Prepend the namespace if the alias doesn't look like it contains a
	// namespace already.
	if !strings.Contains(s, AliasSep) {
		s = namespace + AliasSep + s
	}

	if vlan, ok := v.byAlias[s]; ok {
		return vlan, false, nil
	}

	// Not assigned, allocate a new VLAN
	vlan, err := v.allocate(s)
	return vlan, true, err
}

// allocate a VLAN for the alias. This should only be invoked if the caller has
// acquired the lock for v.
func (v *AllocatedVLANs) allocate(alias string) (int, error) {
	log.Info("creating alias for %v", alias)

	// Find the next unallocated VLAN, taking into account that a range may be
	// specified for the supplied alias.
	r := v.ranges[""] // default
	for prefix, r2 := range v.ranges {
		if strings.HasPrefix(alias, prefix+AliasSep) {
			r = r2
		}
	}

	log.Info("found range: %v", r)

	// Find the next unallocated VLAN
outer:
	for {
		// Look to see if a VLAN is already allocated
		for v.byVLAN[r.Next] != "" {
			r.Next += 1
		}

		// Ensure that we're within the specified bounds
		if r.Next >= r.Max {
			// Ran out of VLANs... oops
			return 0, ErrOutOfVLANs
		}

		// If we're in the default range, make sure we don't allocate anything
		// in a reserved range of VLANs
		if r == v.ranges[""] {
			for prefix, r2 := range v.ranges {
				if prefix == "" {
					continue
				}

				if r.Next >= r2.Min && r.Next < r2.Max {
					r.Next = r2.Max
					continue outer
				}
			}
		}

		// all the checks passed
		break
	}

	log.Info("adding VLAN alias %v => %v", alias, r.Next)

	v.byVLAN[r.Next] = alias
	v.byAlias[alias] = r.Next

	return r.Next, nil
}

// AddAlias sets the VLAN for the provided alias.
func (v *AllocatedVLANs) AddAlias(alias string, vlan int) error {
	v.Lock()
	defer v.Unlock()

	log.Info("adding VLAN alias %v => %v", alias, vlan)

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

// GetVLAN returns the alias for a given VLAN or ErrUnknownVLAN.
func (v *AllocatedVLANs) GetVLAN(alias string) (int, error) {
	v.Lock()
	defer v.Unlock()

	if vlan, ok := v.byAlias[alias]; ok {
		return vlan, nil
	}

	return 0, ErrUnknownVLAN
}

// GetAlias returns the alias for a given VLAN or ErrUnknownAlias. Note that
// previously Blacklisted VLANs will return the const BlacklistedVLAN.
func (v *AllocatedVLANs) GetAlias(vlan int) (string, error) {
	v.Lock()
	defer v.Unlock()

	if alias, ok := v.byVLAN[vlan]; ok {
		return alias, nil
	}

	return "", ErrUnknownAlias
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

// Delete allocation for aliases matching a given prefix. Also clears any
// ranges set for the given prefix.
func (v *AllocatedVLANs) Delete(namespace, prefix string) {
	v.Lock()
	defer v.Unlock()

	// Prepend active namespace if it doesn't look like the user is trying to
	// supply a namespace already.
	if !strings.Contains(prefix, AliasSep) {
		prefix = namespace + AliasSep + prefix
	}

	log.Info("deleting VLAN aliases with prefix: `%v`", prefix)

	for alias, vlan := range v.byAlias {
		if strings.HasPrefix(alias, prefix) {
			delete(v.byVLAN, vlan)
			delete(v.byAlias, alias)
		}
	}

	// Don't delete the default range
	if prefix != AliasSep {
		delete(v.ranges, strings.TrimSuffix(prefix, AliasSep))
	} else {
		// However, do reset the Min/Max ranges
		v.ranges[""].Min = VLANStart
		v.ranges[""].Max = VLANEnd
	}

	// Reset next counter so that we can find the recently freed VLANs
	for _, r := range v.ranges {
		r.Next = r.Min
	}
}

// SetRange reserves a range of VLANs for a particular prefix. VLANs are
// allocated in the range [min, max).
func (v *AllocatedVLANs) SetRange(prefix string, min, max int) error {
	v.Lock()
	defer v.Unlock()

	log.Info("setting range for %v: [%v, %v)", prefix, min, max)

	// Test for conflicts with other ranges
	for prefix2, r := range v.ranges {
		if prefix == prefix2 || prefix2 == "" {
			continue
		}

		if min < r.Max && r.Min <= max {
			return fmt.Errorf("range overlaps with another namespace: %v", prefix2)
		}
	}

	// Warn if we detect any holes in the range
	for i := min; i < max; i++ {
		if _, ok := v.byVLAN[i]; ok {
			log.Warn("detected hole in VLAN range %v -> %v: %v", min, max, i)
		}
	}

	v.ranges[prefix] = &Range{
		Min:  min,
		Max:  max,
		Next: min,
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
	log.Info("blacklisting %v", vlan)

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

// ParseVLAN parses s and returns a VLAN. If s can be parsed as an integer, the
// resulting integer is returned. If s matches an existing alias, that VLAN is
// returned. Otherwise, returns ErrUnknownVLAN.
func (v *AllocatedVLANs) ParseVLAN(namespace, s string) (int, error) {
	v.Lock()
	defer v.Unlock()

	log.Info("parsing vlan: %v namespace: %v", s, namespace)

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
			v.blacklist(vlan)
		}

		return vlan, nil
	}

	// Prepend active namespace if it doesn't look like the user is trying to
	// supply a namespace already.
	if !strings.Contains(s, AliasSep) {
		s = namespace + AliasSep + s
	}

	if vlan, ok := v.byAlias[s]; ok {
		return vlan, nil
	}

	return 0, ErrUnknownVLAN
}

// PrintVLAN prints the alias for the VLAN, if one is set. Will trim off the
// namespace prefix if it matches the currently active namespace.
func (v *AllocatedVLANs) PrintVLAN(namespace string, vlan int) string {
	v.Lock()
	defer v.Unlock()

	if alias, ok := v.byVLAN[vlan]; ok && alias != BlacklistedVLAN {
		// If we're in the namespace identified by the alias, we can trim off
		// the `<namespace>//` prefix.
		parts := strings.Split(alias, AliasSep)
		if namespace == parts[0] {
			alias = strings.Join(parts[1:], AliasSep)
		}

		return fmt.Sprintf("%v (%d)", alias, vlan)
	}

	return strconv.Itoa(vlan)
}

func (v *AllocatedVLANs) Tabular(namespace string) [][]string {
	res := [][]string{}

	for alias, vlan := range v.byAlias {
		parts := strings.Split(alias, AliasSep)
		if namespace != "" && namespace != parts[0] {
			continue
		}

		res = append(res,
			[]string{
				parts[0],
				strings.Join(parts[1:], AliasSep),
				strconv.Itoa(vlan),
			})
	}

	return res
}
