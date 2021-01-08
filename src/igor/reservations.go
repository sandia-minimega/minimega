// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.
package main

import (
	"errors"
	"fmt"
	"math/rand"
	log "minilog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Reservations stores current and future reservations
type Reservations struct {
	M map[uint64]*Reservation

	// dirty set to true when reservations changed
	dirty bool
}

// Find reservation by name
func (r *Reservations) Find(s string) *Reservation {
	for _, res := range r.M {
		if res.Name == s {
			if res.InstallError != "" {
				log.Warn("reservation has install error: %v", res.InstallError)
			}

			return res
		}
	}

	return nil
}

// ActiveHosts returns a map of active hosts at a given time
func (r *Reservations) ActiveHosts(t time.Time) map[string]*Reservation {
	m := map[string]*Reservation{}

	for _, res := range r.M {
		if res.IsActive(t) {
			for _, h := range res.Hosts {
				m[h] = res
			}
		}
	}

	return m
}

// Housekeeping deletes expired reservations and installs newly active
// reservations
func (r *Reservations) Housekeeping() error {
	for _, res := range r.M {
		if res.IsExpired(igor.Now) {
			// Reservation expired; delete it
			if err := r.Delete(res.ID); err != nil {
				return err
			}

			continue
		}

		if !res.IsActive(igor.Now) || res.Installed || res.InstallError != "" {
			// Reservation is in the future, already installed, or errored
			continue
		}

		// attempt install
		if err := r.Install(res); err != nil {
			log.Error("install %v error: %v", res.Name, err)
			res.InstallError = err.Error()
		} else {
			res.Installed = true
		}

		r.dirty = true
	}

	return nil
}

func (r *Reservations) Install(res *Reservation) error {
	// check to see if we need to install the reservation
	if _, err := os.Stat(res.Filename()); err == nil {
		// already installed
		log.Info("%v is already installed", res.Name)

		return nil
	}

	// Vlan wasn't specified by flag
	if res.Vlan == 0 {
		// pick a network segment
		if v, err := r.NextVLAN(); err != nil {
			return fmt.Errorf("error setting network isolation: %v", err)
		} else {
			res.Vlan = v
		}
	}

	// update network config
	if err := networkSet(res.Hosts, res.Vlan); err != nil {
		return fmt.Errorf("error setting network isolation: %v", err)
	}

	if err := igor.Backend.Install(res); err != nil {
		return err
	}

	if igor.Config.AutoReboot {
		if err := doPower(res.Hosts, "cycle"); err != nil {
			// everything should be set, user can try to power cycle
			log.Warn("unable to power cycle %v: %v", res.Name, err)
		}
	}

	emitReservationLog("INSTALL", res)
	return nil
}

// DeleteByName deletes a reservation based on it's name
func (r *Reservations) DeleteByName(s string) error {
	for id, res := range r.M {
		if res.Name == s {
			return r.Delete(id)
		}
	}

	return fmt.Errorf("reservation does not exist: %v", s)
}

// Delete deletes a reservation based on it's ID
func (r *Reservations) Delete(id uint64) error {
	res, ok := r.M[id]
	if !ok {
		// that's strange...
		return errors.New("invalid reservation ID")
	}

	// Only clear network and uninstall if the reservation is installed
	if res.Installed {
		// clean up the network config
		if err := networkClear(res.Hosts); err != nil {
			return fmt.Errorf("error clearing network isolation: %v", err)
		}

		// unset cobbler or TFTP configuration
		if err := igor.Uninstall(res); err != nil {
			return fmt.Errorf("unable to uninstall reservation: %v", err)
		}
	}

	// We use this to indicate if a reservation has been created or not
	// It's used with Cobbler too, even though we don't manually manage PXE files.
	os.Remove(res.Filename())

	if err := r.PurgeFiles(res); err != nil {
		return fmt.Errorf("unable to purge files: %v", err)
	}

	// Finally, purge it from the reservations
	delete(r.M, res.ID)
	r.dirty = true

	emitReservationLog("DELETED", res)

	return nil
}

func (r *Reservations) Edit(res, res2 *Reservation) {
	r.M[res.ID] = res2
	r.dirty = true
}

func (r *Reservations) EditOwner(res *Reservation, owner string) {
	res.Owner = owner
	r.dirty = true
}

func (r *Reservations) EditGroup(res *Reservation, group, gid string) {
	res.Group = group
	res.GroupID = gid
	r.dirty = true
}

// Nodes converts the reservation-centric map into a node centric map
func (r *Reservations) Nodes() map[string][]*Reservation {
	nodes := make(map[string][]*Reservation)

	// pad the map with all the valid hosts
	for _, h := range igor.validHosts() {
		nodes[h] = nil
	}

	// invert the map
	for _, res := range r.M {
		for _, h := range res.Hosts {
			nodes[h] = append(nodes[h], res)
		}
	}

	// sort the reservations for each node so that they are in order based on
	// start time
	for h := range nodes {
		sort.Slice(nodes[h], func(i, j int) bool {
			return nodes[h][i].Start.Before(nodes[h][j].Start)
		})
	}

	return nodes
}

// Schedule finds a start time for res in the existing reservations. Options:
//
//  * Hosts: If blank, a contiguous block of hosts will be found.
//  * Duration: set the duration for the reservation
//  * Start: request scheduling after a given time, zero mean anytime
//
// Outputs (via res):
//
//  * ID: Reservation ID
//  * Hosts: set to list of hosts
//  * Start: set to valid start time
//  * End: set to valid end time
//
// If speculative is true, the reservation is not added to the known
// reservations
func (r *Reservations) Schedule(res *Reservation, speculative bool) error {
	if len(res.Hosts) == 0 {
		return errors.New("reservation must include at least one host")
	}

	var err error
	if res.Hosts[0] != "" {
		err = scheduleHosts(r, res)
	} else {
		err = scheduleContiguous(r, res)
	}

	if err != nil {
		return err
	}

	if speculative {
		return nil
	}

	res.ID = uint64(rand.Int63())

	r.M[res.ID] = res
	r.dirty = true

	return nil
}

func (r *Reservations) Extend(res *Reservation, d time.Duration) error {
	// try to schedule a dummy reservation on the same set of nodes and see if
	// we're able to schedule immediately after the existing reservation
	res2 := &Reservation{
		Hosts:    res.Hosts,
		Start:    res.End,
		Duration: d,
	}

	if err := scheduleHosts(r, res2); err != nil {
		return err
	}

	if res2.Start != res.End {
		return errors.New("nodes unavailable to extend")
	}

	res.End = res.End.Add(d)
	res.Duration += d

	r.dirty = true
	return nil
}

func (r *Reservations) UsingVLAN(vlan int) []*Reservation {
	rs := []*Reservation{}
	for _, res := range r.M {
		if vlan == res.Vlan {
			rs = append(rs, res)
		}
	}
	return rs
}

func (r *Reservations) NextVLAN() (int, error) {
OuterLoop:
	for i := igor.VLANMin; i <= igor.VLANMax; i++ {
		for _, res := range r.M {
			if i == res.Vlan {
				continue OuterLoop
			}
		}

		return i, nil
	}

	return 0, errors.New("no vlans available")
}

// PurgeFiles removes the KernelHash/InitrdHash if they are not used by any
// other reservations.
func (r *Reservations) PurgeFiles(res *Reservation) error {
	// If no other reservations are using them, delete the kernel and/or
	// initrd. Make sure not to include ourselves if the reservation is still
	// in the list of reservations.
	var kfound, ifound bool
	for _, res2 := range r.M {
		if res2.KernelHash == res.KernelHash && res.ID != res2.ID {
			kfound = true
		}
		if res2.InitrdHash == res.InitrdHash && res.ID != res2.ID {
			ifound = true
		}
	}

	if !kfound && res.KernelHash != "" {
		fname := filepath.Join(igor.Config.TFTPRoot, "igor", res.KernelHash+"-kernel")
		if err := os.Remove(fname); err != nil {
			return err
		}
	}

	if !ifound && res.InitrdHash != "" {
		fname := filepath.Join(igor.Config.TFTPRoot, "igor", res.InitrdHash+"-initrd")
		if err := os.Remove(fname); err != nil {
			return err
		}
	}

	return nil
}
