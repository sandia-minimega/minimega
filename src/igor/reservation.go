// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
package main

import (
	"fmt"
	log "minilog"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

// Represents a single reservation
type Reservation struct {
	ID      uint64
	ResName string

	Owner   string
	Group   string // optional group associated with reservation
	GroupID string // resolved group ID for Group

	StartTime int64   // UNIX time
	EndTime   int64   // UNIX time
	Duration  float64 // minutes

	Hosts    []string // separate, not a range
	PXENames []string // eg C000025B

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
	return filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", "igor", r.ResName)
}

// IsActive returns true if the reservation is active at the given time
func (r Reservation) IsActive(t time.Time) bool {
	start := time.Unix(r.StartTime, 0)
	end := time.Unix(r.EndTime, 0)

	return start.Before(t) && end.After(t)
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

func (r *Reservation) SetKernel(k string) error {
	var err error

	dir := filepath.Join(igorConfig.TFTPRoot, "igor")

	r.Kernel = k
	if r.KernelHash, err = install(k, dir, "-kernel"); err != nil {
		return fmt.Errorf("install kernel failed: %v", err)
	}

	return nil
}

func (r *Reservation) SetInitrd(i string) error {
	var err error

	dir := filepath.Join(igorConfig.TFTPRoot, "igor")

	r.Initrd = i
	if r.InitrdHash, err = install(i, dir, "-initrd"); err != nil {
		return fmt.Errorf("install initrd failed: %v", err)
	}

	return nil
}

// SetKernelInitrd wraps calling SetKernel and SetInitrd, calling PurgeFiles if
// SetInitrd fails and we have already succesfully call SetKernel to avoid
// leaking a kernel.
func (r *Reservation) SetKernelInitrd(k, i string) error {
	if err := r.SetKernel(k); err != nil {
		return err
	}

	if err := r.SetInitrd(i); err != nil {
		// clean up already installed kernel
		if err := r.PurgeFiles(); err != nil {
			log.Error("leaked kernel: %v", k)
		}

		return err
	}

	return nil
}

// purgeFiles removes the KernelHash/InitrdHash if they are not used by any
// other reservations.
func (r *Reservation) PurgeFiles() error {
	// If no other reservations are using them, delete the kernel and/or initrd
	var kfound, ifound bool
	for _, r2 := range Reservations {
		if r2.KernelHash == r.KernelHash {
			kfound = true
		}
		if r2.InitrdHash == r.InitrdHash {
			ifound = true
		}
	}

	if !kfound && r.KernelHash != "" {
		fname := filepath.Join(igorConfig.TFTPRoot, "igor", r.KernelHash+"-kernel")
		if err := os.Remove(fname); err != nil {
			return err
		}
	}

	if !ifound && r.InitrdHash != "" {
		fname := filepath.Join(igorConfig.TFTPRoot, "igor", r.InitrdHash+"-initrd")
		if err := os.Remove(fname); err != nil {
			return err
		}
	}

	return nil
}
