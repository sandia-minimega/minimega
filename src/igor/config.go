// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"io/ioutil"
	log "minilog"
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

	// ConcurrencyLimit is the maximum number of parallel commands igor
	// executes for cobbler and power management.
	ConcurrencyLimit uint

	// CommandRetries is the maximum number of times igor will rerun a command
	// that fails.
	CommandRetries uint

	// Domain for email address
	Domain string
}

// Read in the configuration from the specified path.
func readConfig(path string) (c Config) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal("Couldn't read config file: %v", err)
	}

	err = json.Unmarshal(b, &c)
	if err != nil {
		log.Fatal("Couldn't parse json: %v", err)
	}
	return
}
