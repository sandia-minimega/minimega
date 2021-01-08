// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.
package main

import (
	"fmt"
	log "minilog"
	"net"
	"os/user"
	"path/filepath"
	"time"
)

// Reservation stores the information about a single reservation
type Reservation struct {
	ID   uint64
	Name string

	Owner   string
	Group   string // optional group associated with reservation
	GroupID string // resolved group ID for Group

	Start    time.Time
	End      time.Time
	Duration time.Duration

	Hosts    []string // separate, not a range
	PXENames []string // e.g. C000025B

	CobblerProfile string // Optional; if set, use this Cobbler profile instead of a kernel+initrd
	KernelArgs     string
	Kernel         string
	Initrd         string
	KernelHash     string
	InitrdHash     string

	Vlan int

	// Installed is set when the reservation is first installed
	Installed bool
	// InstallError is set when the reservation failed to install
	InstallError string
}

// Filename returns the filename that stores the reservation configuration
func (r Reservation) Filename() string {
	return filepath.Join(igor.TFTPRoot, "pxelinux.cfg", "igor", r.Name)
}

// IsActive returns true if the reservation is active at the given time
func (r Reservation) IsActive(t time.Time) bool {
	return r.Start.Before(t) && r.End.After(t)
}

// IsOverlap tests whether the reservation overlaps with a given time range
func (r Reservation) IsOverlap(start, end time.Time) bool {
	return r.End.Sub(start) > 0 && r.Start.Sub(end) < 0
}

// Remaining returns how long the reservation has remaining at the given time
// if the reservation is active. If the reservation is not active, it returns
// how long the reservation will be active for.
func (r Reservation) Remaining(t time.Time) time.Duration {
	if r.IsActive(t) {
		return r.End.Sub(t)
	}

	return r.Duration
}

// IsExpired returns true if the reservation is before the given time
func (r Reservation) IsExpired(t time.Time) bool {
	return r.End.Before(t)
}

// IsWritable returns true if the reservation can be modified by the given user
func (r Reservation) IsWritable(u *user.User) bool {
	if u.Username == "root" || u.Username == r.Owner {
		return true
	}

	// no group associated with reservations
	if r.Group == "" {
		return false
	}

	groups, err := u.GroupIds()
	if err != nil {
		log.Error("unable to query groups: %v", err)
		// safety first
		return false
	}

	for _, gid := range groups {
		if gid == r.GroupID {
			return true
		}
	}

	return false
}

// SetHosts sets Hosts and PXENames based on IP lookups for the provided hosts.
func (r *Reservation) SetHosts(hosts []string) error {
	log.Info("setting hosts to %v", hosts)

	r.Hosts = hosts

	// First, go from node name to PXE filename
	for _, h := range hosts {
		ips, err := net.LookupIP(h)
		if err != nil {
			return fmt.Errorf("failure looking up %v: %v", h, err)
		}

		for _, ip := range ips {
			pxe := toPXE(ip)
			log.Debug("resolved %v to %v (%v)", h, ip, pxe)
			if pxe != "" {
				r.PXENames = append(r.PXENames, pxe)
			}
		}
	}

	if len(r.PXENames) < len(r.Hosts) {
		log.Error("failed to resolve all node names, possible dns issue")
	}

	return nil
}

func (r *Reservation) SetKernel(k string) error {
	var err error

	dir := filepath.Join(igor.TFTPRoot, "igor")

	r.Kernel = k
	if r.KernelHash, err = install(k, dir, "-kernel"); err != nil {
		return fmt.Errorf("install kernel failed: %v", err)
	}

	return nil
}

func (r *Reservation) SetInitrd(i string) error {
	var err error

	dir := filepath.Join(igor.TFTPRoot, "igor")

	r.Initrd = i
	if r.InitrdHash, err = install(i, dir, "-initrd"); err != nil {
		return fmt.Errorf("install initrd failed: %v", err)
	}

	return nil
}

// Flags returns a string to indicate reservation status
func (r *Reservation) Flags(t time.Time) (flags string) {
	if r.IsActive(t) {
		flags += "A"
	}
	if r.IsWritable(igor.User) {
		flags += "W"
	}
	if r.Installed {
		flags += "I"
	}
	if r.InstallError != "" {
		flags += "E"
	}

	return
}
