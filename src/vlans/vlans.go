// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vlans

import (
	"errors"
	"fmt"
	log "minilog"
	"strconv"
	"strings"
	"sync"
)

// BlacklistedVLAN is used to track manually reserved VLANs
const BlacklistedVLAN = "BLACKLISTED"

// DisconnectedVLAN always resolves to VLAN -1
const DisconnectedVLAN = "DISCONNECTED"

// VLANStart and VLANEnd are the default ranges for VLAN allocation
const VLANStart, VLANEnd = 101, 4096

var ErrUnallocated = errors.New("unallocated")
var ErrOutOfVLANs = errors.New("out of VLANs")

var Default = NewVLANs()

type Range struct {
	Min, Max, Next int
}

// VLANs stores the state for the VLANs that we've allocated so far
type VLANs struct {
	byVLAN  map[int]Alias
	byAlias map[Alias]int

	ranges map[string]*Range

	sync.Mutex
}

func NewVLANs() *VLANs {
	return &VLANs{
		byVLAN:  make(map[int]Alias),
		byAlias: make(map[Alias]int),
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
func (v *VLANs) Allocate(namespace, alias string) (int, bool, error) {
	v.Lock()
	defer v.Unlock()

	a := ParseAlias(namespace, alias)

	if vlan, ok := v.byAlias[a]; ok {
		return vlan, false, nil
	}

	// Not assigned, allocate a new VLAN
	vlan, err := v.allocate(a)
	return vlan, true, err
}

// allocate a VLAN for the alias. This should only be invoked if the caller has
// acquired the lock for v.
func (v *VLANs) allocate(a Alias) (int, error) {
	log.Debug("creating alias for %v", a)

	// Find the next unallocated VLAN, taking into account that a range may be
	// specified for the supplied alias.
	r := v.ranges[""] // default
	for namespace, r2 := range v.ranges {
		if a.Namespace == namespace {
			r = r2
		}
	}

	log.Debug("found range for alias %v: %v", a, r)

	// Find the next unallocated VLAN
outer:
	for {
		// Look to see if a VLAN is already allocated
		for _, ok := v.byVLAN[r.Next]; ok; _, ok = v.byVLAN[r.Next] {
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

	log.Info("adding VLAN alias %v => %v", a, r.Next)

	v.byVLAN[r.Next] = a
	v.byAlias[a] = r.Next

	return r.Next, nil
}

// AddAlias sets the VLAN for the provided alias.
func (v *VLANs) AddAlias(namespace, alias string, vlan int) error {
	v.Lock()
	defer v.Unlock()

	a := ParseAlias(namespace, alias)

	log.Info("adding VLAN alias %v => %v", alias, vlan)

	if _, ok := v.byAlias[a]; ok {
		return errors.New("alias already in use")
	}
	if _, ok := v.byVLAN[vlan]; ok {
		return errors.New("vlan already in use")
	}

	v.byVLAN[vlan] = a
	v.byAlias[a] = vlan

	return nil
}

// GetVLAN returns the VLAN for a given alias or ErrUnallocated.
func (v *VLANs) GetVLAN(namespace, alias string) (int, error) {
	v.Lock()
	defer v.Unlock()

	a := ParseAlias(namespace, alias)

	if vlan, ok := v.byAlias[a]; ok {
		return vlan, nil
	}

	return 0, ErrUnallocated
}

// GetAlias returns the alias for a given VLAN or ErrUnallocated. Note that
// previously Blacklisted VLANs will return the const BlacklistedVLAN.
func (v *VLANs) GetAlias(vlan int) (Alias, error) {
	v.Lock()
	defer v.Unlock()

	if a, ok := v.byVLAN[vlan]; ok {
		return a, nil
	}

	return Alias{}, ErrUnallocated
}

// GetAliases returns a list of aliases with the given prefix.
func (v *VLANs) GetAliases(prefix string) []string {
	v.Lock()
	defer v.Unlock()

	res := []string{}
	for a := range v.byAlias {
		if strings.HasPrefix(a.String(), prefix) {
			res = append(res, a.Value)
		}
	}

	return res
}

// Delete allocation for aliases matching a given namespace. Also clears any
// ranges set for the given namespace if the prefix is empty.
func (v *VLANs) Delete(namespace, prefix string) {
	v.Lock()
	defer v.Unlock()

	log.Info("deleting VLAN for %v//%v", namespace, prefix)

	for a, vlan := range v.byAlias {
		if !strings.HasPrefix(a.Value, prefix) {
			continue
		}

		if a.Namespace == namespace {
			delete(v.byVLAN, vlan)
			delete(v.byAlias, a)
		}
	}

	if prefix != "" {
		return
	}

	// never delete the default range
	if namespace != "" {
		delete(v.ranges, namespace)
	} else {
		v.ranges[""].Min = VLANStart
		v.ranges[""].Max = VLANEnd
	}

	// Reset next counter so that we can find the recently freed VLANs
	for _, r := range v.ranges {
		r.Next = r.Min
	}
}

// SetRange reserves a range of VLANs for a particular namespace. VLANs are
// allocated in the range [min, max).
func (v *VLANs) SetRange(namespace string, min, max int) error {
	v.Lock()
	defer v.Unlock()

	log.Info("setting range for %v: [%v, %v)", namespace, min, max)

	// Test for conflicts with other ranges
	for namespace2, r := range v.ranges {
		if namespace == namespace2 || namespace2 == "" {
			continue
		}

		if min < r.Max && r.Min <= max {
			return fmt.Errorf("range overlaps with another namespace: %v", namespace2)
		}
	}

	// Warn if we detect any holes in the range
	for i := min; i < max; i++ {
		if _, ok := v.byVLAN[i]; ok {
			log.Warn("detected hole in VLAN range %v -> %v: %v", min, max, i)
		}
	}

	v.ranges[namespace] = &Range{
		Min:  min,
		Max:  max,
		Next: min,
	}

	return nil
}

// GetRanges returns a copy of the ranges currently in use.
func (v *VLANs) GetRanges() map[string]Range {
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
func (v *VLANs) Blacklist(vlan int) {
	v.Lock()
	defer v.Unlock()

	v.blacklist(vlan)
}

// blacklist the VLAN. This should only be invoked if the caller has acquired
// the lock for v.
func (v *VLANs) blacklist(vlan int) {
	log.Info("blacklisting %v", vlan)

	if a, ok := v.byVLAN[vlan]; ok {
		delete(v.byAlias, a)
	}
	v.byVLAN[vlan] = Alias{Value: BlacklistedVLAN}
}

// GetBlacklist returns a list of VLANs that have been blacklisted.
func (v *VLANs) GetBlacklist() []int {
	v.Lock()
	defer v.Unlock()

	res := []int{}
	for vlan, alias := range v.byVLAN {
		if alias.Value == BlacklistedVLAN {
			res = append(res, vlan)
		}
	}

	return res
}

// ParseVLAN parses s and returns a VLAN. If s can be parsed as an integer, the
// resulting integer is returned. If s matches an existing alias, that VLAN is
// returned. Otherwise, returns ErrUnallocated.
func (v *VLANs) ParseVLAN(namespace, s string) (int, error) {
	v.Lock()
	defer v.Unlock()

	log.Debug("parsing vlan: %v namespace: %v", s, namespace)

	a := ParseAlias(namespace, s)

	vlan, err := strconv.Atoi(a.Value)
	if err == nil {
		// Check to ensure that VLAN is sane
		if vlan < 0 || vlan >= 4096 {
			return 0, errors.New("invalid VLAN (0 <= vlan < 4096)")
		}

		if alias, ok := v.byVLAN[vlan]; ok && alias.Value != BlacklistedVLAN {
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

	if vlan, ok := v.byAlias[a]; ok {
		return vlan, nil
	}

	return 0, ErrUnallocated
}

// PrintVLAN prints the alias for the VLAN, if one is set. Will trim off the
// namespace prefix if it matches the currently active namespace.
func (v *VLANs) PrintVLAN(namespace string, vlan int) string {
	v.Lock()
	defer v.Unlock()

	if alias, ok := v.byVLAN[vlan]; ok && alias.Value != BlacklistedVLAN {
		// If we're in the namespace identified by the alias, we can trim off
		// the `<namespace>//` prefix.
		s := alias.String()
		if alias.Namespace == namespace {
			s = alias.Value
		}

		return fmt.Sprintf("%v (%d)", s, vlan)
	}

	return strconv.Itoa(vlan)
}

func (v *VLANs) Tabular(namespace string) [][]string {
	res := [][]string{}

	for alias, vlan := range v.byAlias {
		var s string

		// if we're matching on namespace, we should trim that from the alias
		// that we return
		if namespace == "" {
			s = alias.String()
		} else if namespace == alias.Namespace {
			s = alias.Value
		} else {
			continue
		}

		res = append(res,
			[]string{
				s,
				strconv.Itoa(vlan),
			})
	}

	return res
}

func Allocate(namespace, s string) (int, bool, error) {
	return Default.Allocate(namespace, s)
}
func AddAlias(namespace, alias string, vlan int) error {
	return Default.AddAlias(namespace, alias, vlan)
}
func GetVLAN(namespace, alias string) (int, error) {
	return Default.GetVLAN(namespace, alias)
}
func GetAlias(vlan int) (Alias, error) {
	return Default.GetAlias(vlan)
}
func GetAliases(prefix string) []string {
	return Default.GetAliases(prefix)
}
func Delete(namespace, alias string) {
	Default.Delete(namespace, alias)
}
func SetRange(prefix string, min, max int) error {
	return Default.SetRange(prefix, min, max)
}
func GetRanges() map[string]Range {
	return Default.GetRanges()
}
func Blacklist(vlan int) {
	Default.Blacklist(vlan)
}
func GetBlacklist() []int {
	return Default.GetBlacklist()
}
func ParseVLAN(namespace, s string) (int, error) {
	return Default.ParseVLAN(namespace, s)
}
func PrintVLAN(namespace string, vlan int) string {
	return Default.PrintVLAN(namespace, vlan)
}
func Tabular(namespace string) [][]string {
	return Default.Tabular(namespace)
}
