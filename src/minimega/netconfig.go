// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"io"
	log "minilog"
	"net"
	"strings"

	"github.com/google/gopacket/macs"
)

var validMACPrefix [][3]byte

func init() {
	for k, _ := range macs.ValidMACPrefixMap {
		validMACPrefix = append(validMACPrefix, k)
	}
}

// NetConfig contains all the network-related config for an interface. The IP
// addresses are automagically populated by snooping ARP traffic. The bandwidth
// stats and IP addresses are updated on-demand by calling the UpdateNetworks
// function of BaseConfig.
type NetConfig struct {
	Alias  string
	VLAN   int
	Bridge string
	Tap    string
	MAC    string
	Driver string
	IP4    string
	IP6    string

	RxRate, TxRate float64 // Most recent bandwidth measurements for Tap

	// Raw string that we used when creating this network config will be
	// reparsed if we ever clone the VM that has this config.
	Raw string
}

type NetConfigs []NetConfig

func NewVMConfig() VMConfig {
	c := VMConfig{}
	c.Clear(Wildcard)
	return c
}

// ParseNetConfig processes the input specifying the bridge, VLAN alias, and
// mac for one interface to a VM and updates the vm config accordingly. The
// VLAN alias must be resolved using the active namespace. This takes a bit of
// parsing, because the entry can be in a few forms:
//
// 	vlan alias
//
//	vlan alias,mac
//	bridge,vlan alias
//	vlan alias,driver
//
//	bridge,vlan alias,mac
//	vlan alias,mac,driver
//	bridge,vlan alias,driver
//
//	bridge,vlan alias,mac,driver
//
// If there are 2 or 3 fields, just the last field for the presence of a mac
func ParseNetConfig(spec string, nics map[string]bool) (*NetConfig, error) {
	// example: my_bridge,100,00:00:00:00:00:00
	f := strings.Split(spec, ",")

	isDriver := func(d string) bool {
		return nics[d]
	}

	var b, v, m, d string

	switch len(f) {
	case 1:
		v = f[0]
	case 2:
		if isMAC(f[1]) {
			// vlan, mac
			v, m = f[0], f[1]
		} else if isDriver(f[1]) {
			// vlan, driver
			v, d = f[0], f[1]
		} else {
			// bridge, vlan
			b, v = f[0], f[1]
		}
	case 3:
		if isMAC(f[2]) {
			// bridge, vlan, mac
			b, v, m = f[0], f[1], f[2]
		} else if isMAC(f[1]) && isDriver(f[2]) {
			// vlan, mac, driver
			v, m, d = f[0], f[1], f[2]
		} else if isDriver(f[2]) {
			// bridge, vlan, driver
			b, v, d = f[0], f[1], f[2]
		} else {
			return nil, errors.New("malformed netspec")
		}
	case 4:
		if isMAC(f[2]) && isDriver(f[3]) {
			b, v, m, d = f[0], f[1], f[2], f[3]
		} else {
			return nil, errors.New("malformed netspec")
		}
	default:
		return nil, errors.New("malformed netspec")
	}

	log.Info(`got bridge="%v", alias="%v", mac="%v", driver="%v"`, b, v, m, d)

	if b == "" {
		b = DefaultBridge
	}

	if d == "" {
		d = DefaultKVMDriver
	}

	return &NetConfig{
		Alias:  v,
		Bridge: b,
		MAC:    strings.ToLower(m),
		Driver: d,
	}, nil
}

// String representation of NetConfig, should be able to parse back into a
// NetConfig.
func (c NetConfig) String() string {
	parts := []string{}

	if c.Bridge != "" && c.Bridge != DefaultBridge {
		parts = append(parts, c.Bridge)
	}

	parts = append(parts, c.Alias)

	if c.MAC != "" {
		parts = append(parts, c.MAC)
	}

	if c.Driver != "" && c.Driver != DefaultKVMDriver {
		parts = append(parts, c.Driver)
	}

	return strings.Join(parts, ",")
}

func (c NetConfigs) String() string {
	parts := []string{}
	for _, n := range c {
		parts = append(parts, n.String())
	}

	return strings.Join(parts, " ")
}

func (c NetConfigs) WriteConfig(w io.Writer) error {
	if len(c) > 0 {
		_, err := fmt.Fprintf(w, "vm config networks %v\n", c)
		return err
	}

	return nil
}

func isMAC(mac string) bool {
	_, err := net.ParseMAC(mac)
	return err == nil
}

func isAllocatedMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return false
	}

	_, allocated := macs.ValidMACPrefixMap[[3]byte{hw[0], hw[1], hw[2]}]
	return allocated
}
