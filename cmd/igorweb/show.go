package main

import (
	"os/user"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"github.com/sandia-minimega/minimega/v2/pkg/ranges"
)

// Show stores data received from "igor show -json"
type Show struct {
	Error                                       string
	LastUpdated                                 time.Time
	Prefix                                      string
	RangeStart, RangeEnd, RackWidth, RackHeight int
	Available, Down                             []string
	Reservations                                []Res
	Listimages                                  map[string]*kiPair
	Path                                        string
}

// Returns the range of nodes based on Show
func (s Show) Range() *ranges.Range {
	rnge, err := ranges.NewRange(s.Prefix, s.RangeStart, s.RangeEnd)
	if err != nil {
		log.Fatal("Cannot compute Range: %v", err)
	}
	return rnge
}

// Like Show, but includes a username, so we can personalize what the
// user sees
type UserShow struct {
	*Show
	Username string
}

// User returns the user.User for the UserShow's set username
func (s UserShow) User() *user.User {
	u, err := user.Lookup(s.Username)
	if err != nil {
		log.Warn("Unable to lookup user \"%s\": %v", s.Username, err)
		return nil
	}
	return u
}

// DownRow returns a ResTableRow that enumerates all nodes in the "down" state
func (s UserShow) DownRow() ResTableRow {
	return ResTableRow{
		StartInt: time.Now().Unix(),
		EndInt:   0,
		Nodes:    s.Range().RangeToInts(s.Down),
	}
}

// ResTable returns the reservations as a ResTable. The first element
// in the ResTable enumerates the hosts that are "down"
func (s *UserShow) ResTable() ResTable {
	// First element is a row containing all down nodes
	resRows := ResTable{s.DownRow()}
	for _, r := range s.Reservations {
		resRows = append(resRows, r.ResTableRow(s))
	}
	return resRows
}

// Res represents a Reservation. It's for unmarshalling
// igor.Reservations from "igor show -json". However, the number of
// fields in Res is less than igor.Reservation, since we don't need
// everything there.
type Res struct {
	Name           string
	Owner          string
	Group          string
	Kernel         string
	Initrd         string
	CobblerProfile string
	KernelArgs     string
	Start          time.Time
	End            time.Time
	Hosts          []string // separate, not a range
}

// GetGroup returns the user.Group that can edit tihs
// reservation. Returns nil if there's no group set
func (r Res) GetGroup() *user.Group {
	if r.Group == "" {
		return nil
	}

	g, err := user.LookupGroup(r.Group)
	if err != nil {
		log.Warn("Unable to lookup group \"%s\": %v", r.Group, err)
		return nil
	}
	return g
}

// IsEditableBy returns true if the user u is able to edit this
// reservation (r)
func (r Res) IsEditableBy(u *user.User) bool {
	if u == nil {
		return false
	}

	// If you own it, you can edit it
	if u.Username == r.Owner {
		return true
	}

	// Find the group that owns this reservation
	group := r.GetGroup()
	if group == nil {
		return false
	}

	// Find the groups that this user is a member of
	groups, err := u.GroupIds()
	if err != nil {
		log.Warn("Unable to list group IDs for user \"%s\": %v", u.Username, err)
		return false
	}

	// See if the user is a member of the reservation's group
	for _, g := range groups {
		if g == group.Gid {
			return true
		}
	}
	return false
}

// ResTableRow returns a Res as a ResTableRow.
func (r Res) ResTableRow(s *UserShow) ResTableRow {
	timefmt := "Jan 2 15:04"

	return ResTableRow{
		Name:           r.Name,
		Owner:          r.Owner,
		Group:          r.Group,
		CanEdit:        r.IsEditableBy(s.User()),
		Kernel:         r.Kernel,
		Initrd:         r.Initrd,
		CobblerProfile: r.CobblerProfile,
		KernelArgs:     r.KernelArgs,
		Start:          r.Start.Format(timefmt),
		StartInt:       r.Start.UnixNano(),
		End:            r.End.Format(timefmt),
		EndInt:         r.End.UnixNano(),
		Nodes:          s.Range().RangeToInts(r.Hosts),
	}
}
