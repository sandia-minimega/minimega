// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"fmt"
	"math"
	log "minilog"
	"os"
	"ranges"
	"strconv"
	"syscall"
	"time"
)

// The configuration of the system
type Config struct {
	// TFTPRoot is where the igor configs are stored.
	// It should be the root of your TFTP server if not using Cobbler
	// If using Cobbler, it should be /var/lib/igor
	TFTPRoot string
	// The prefix for cluster nodes, e.g. 'kn' if nodes are named kn01 etc.
	Prefix string
	// The first node number in the cluster, (usually 1)
	Start int
	// The last node number in the cluster
	End int
	// How wide the numeric part of a node name must be padded.
	// If you have a node named kn001, set Padlen to 3
	// If you have one named kn1, set it to 0.
	Padlen int
	// Width and height of each rack in the cluster. Only used for display purposes
	Rackwidth  int
	Rackheight int
	// printf-formatted string to power on/off a single node
	// e.g. "powerman on %s"
	PowerOnCommand  string
	PowerOffCommand string
	// True if using Cobbler to manage nodes
	UseCobbler bool
	// If using Cobbler, nodes not in a reservation will be set to this profile
	CobblerDefaultProfile string
	// If set to true, nodes will be automatically rebooted when
	// the reservation starts, if possible
	AutoReboot bool
	// VLAN segmentation options
	// VLANMin/VLANMax: specify a range of VLANs to use
	// NodeMap: maps hostnames to switch port names
	// Network: selects which type of switch is in use. Set to "" to disable VLAN segmentation
	// NetworkUser/NetworkPassword: login info for a switch user capable of configuring ports
	// NetworkURL: HTTP URL for sending API commands to the switch
	VLANMin         int               `json:"vlan_min"`
	VLANMax         int               `json:"vlan_max"`
	NodeMap         map[string]string `json:"node_map"`
	Network         string
	NetworkUser     string
	NetworkPassword string
	NetworkURL      string `json:"network_url"`
	// Set this to a DNS server if multiple servers are available and hostname lookups are failing
	DNSServer string
	// A file to receive log info
	LogFile string
	// NodeLimit: max nodes a non-root user can reserve
	// TimeLimit: max time a non-root user can reserve
	NodeLimit int
	TimeLimit int

	// ExtendWithin is the number of minutes before the end of a reservation
	// that it can be extended. For example, 24*60 would mean that the
	// reservation can be extended within 24 hours of its expiration.
	ExtendWithin int

	// ConcurrencyLimit is the maximum number of parallel commands igor
	// executes for cobbler and power management.
	ConcurrencyLimit uint

	// CommandRetries is the maximum number of times igor will rerun a command
	// that fails.
	CommandRetries uint

	// Domain for email address
	Domain string

	//Igor Notify Notice time (in minutes) is the amount of time before reservation expires users are notified
	ExpirationLeadTime int

	// Pause is set by administrators to prevent users from
	// creating new reservations or extending current
	// reservations. If the value is not "", igor is paused.
	Pause string
}

// Read in the configuration from the specified path. Checks to make sure that
// the config is owned and only writable by the effective user to ensure that
// users can't try to specify their own config when we're running with setuid.
func readConfig(path string) (c Config) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal("unable to open config file: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		log.Fatal("unable to stat config file: %v", err)
	}

	switch fi := fi.Sys().(type) {
	case *syscall.Stat_t:
		euid := syscall.Geteuid()
		if fi.Uid != uint32(euid) {
			log.Fatal("config file must be owned by running user")
		}

		if fi.Mode&0022 != 0 {
			log.Fatal("config file must only be writable by running user")
		}

		if fi.Mode&0044 != 0 {
			log.Fatal("config file must only be readable by running user")
		}
	default:
		log.Warn("unable to check config ownership/permissions")
	}

	if err := json.NewDecoder(f).Decode(&c); err != nil {
		log.Fatal("unable to parse json: %v", err)
	}

	return
}

func (c Config) validHosts() []string {
	fmtstring := "%s%0" + strconv.Itoa(c.Padlen) + "d"

	names := []string{}
	for i := c.Start; i <= c.End; i++ {
		names = append(names, fmt.Sprintf(fmtstring, c.Prefix, i))
	}
	return names
}

func (c Config) unsplitRange(hosts []string) string {
	r, _ := ranges.NewRange(c.Prefix, c.Start, c.End)
	s, _ := r.UnsplitRange(hosts)

	return s
}

func (c Config) splitRange(s string) []string {
	r, _ := ranges.NewRange(c.Prefix, c.Start, c.End)
	v, _ := r.SplitRange(s)

	return v
}

func (c Config) checkTimeLimit(nodes int, d time.Duration) error {
	// no time limit in the config
	if c.TimeLimit <= 0 {
		return nil
	}

	max := time.Duration(c.TimeLimit) * time.Minute
	if nodes > 1 {
		// compute the max reservation length for this many nodes, rounding up
		// to the nearest minute.
		max = time.Duration(float64(max)/math.Log2(float64(nodes)) + 0.5)
	}

	if d > max {
		return fmt.Errorf("max allowable duration for %v nodes is %v (you requested %v)", nodes, max, d)
	}

	return nil
}
