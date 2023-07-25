// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

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
	QinQ   bool

	RxRate, TxRate float64 // Most recent bandwidth measurements for Tap

	// Raw string that we used when creating this network config will be
	// reparsed if we ever clone the VM that has this config.
	Raw string
}

type NetConfigs []NetConfig

// ParseNetConfig processes the input specifying the bridge, VLAN alias, and
// mac for one interface to a VM and updates the vm config accordingly. The
// VLAN alias must be resolved using the active namespace. This takes a bit of
// parsing, because the entry can be in a few forms:
//
//	vlan alias
//
//	bridge,vlan alias
//	vlan alias,mac
//	vlan alias,driver
//	vlan alias,driver,qinq
//
//	bridge,vlan alias,mac
//	bridge,vlan alias,driver
//	bridge,vlan alias,qinq
//	vlan alias,mac,driver
//	vlan alias,mac,qinq
//	vlan alias,driver,qinq
//
//	bridge,vlan alias,mac,driver
//	bridge,vlan alias,mac,qinq
//	bridge,vlan alias,driver,qinq
//	vlan alias,mac,driver,qinq
//
//	bridge,vlan alias,mac,driver,qinq
//
// If there are 2 or 3 fields, just the last field for the presence of a mac
func ParseNetConfig(spec string, nics map[string]bool) (*NetConfig, error) {
	// example: my_bridge,100,00:00:00:00:00:00
	f := strings.Split(spec, ",")

	isDriver := func(d string) bool {
		return nics[d]
	}

	isQinQ := func(q string) bool {
		return strings.EqualFold(q, "qinq")
	}

	var b, v, m, d string
	var q bool

	switch len(f) {
	case 1:
		v = f[0]
	case 2:
		if isQinQ(f[1]) {
			// vlan, qinq
			v, q = f[0], true
		} else if isMAC(f[1]) {
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
		if isQinQ(f[2]) && isMAC(f[1]) {
			// vlan, mac, qinq
			v, m, q = f[0], f[1], true
		} else if isQinQ(f[2]) && isDriver(f[1]) {
			// vlan, driver, qinq
			v, d, q = f[0], f[1], true
		} else if isQinQ(f[2]) {
			// bridge, vlan, qinq
			b, v, q = f[0], f[1], true
		} else if isMAC(f[2]) {
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
		if isQinQ(f[3]) && isMAC(f[1]) {
			// vlan, mac, driver, qinq
			v, m, d, q = f[0], f[1], f[2], true
		} else if isQinQ(f[3]) && isMAC(f[2]) {
			// bridge, vlan, mac, qinq
			b, v, m, q = f[0], f[1], f[2], true
		} else if isQinQ(f[3]) && isDriver(f[2]) {
			// bridge, vlan, driver, qinq
			b, v, d, q = f[0], f[1], f[2], true
		} else if isDriver(f[3]) && isMAC(f[2]) {
			// bridge, vlan, mac, driver
			b, v, m, d = f[0], f[1], f[2], f[3]
		} else {
			return nil, errors.New("malformed netspec")
		}
	case 5:
		if isMAC(f[2]) && isDriver(f[3]) && isQinQ(f[4]) {
			b, v, m, d, q = f[0], f[1], f[2], f[3], true
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
		QinQ:   q,
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

	if c.QinQ {
		parts = append(parts, "qinq")
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

// BondConfig contains all the bond-related configs for a bond.
type BondConfig struct {
	Name     string
	Mode     string
	LACP     string
	Fallback bool
	QinQ     bool

	Interfaces []int
	Bridge     string
	VLAN       int
	Raw        string

	created bool
}

type BondConfigs []BondConfig

func (old BondConfig) Copy() BondConfig {
	// copy all fields
	cfg := old

	// deep copy slices
	cfg.Interfaces = make([]int, len(old.Interfaces))
	copy(cfg.Interfaces, old.Interfaces)

	return cfg
}

func (c BondConfig) Contains(index int) bool {
	for _, idx := range c.Interfaces {
		if idx == index {
			return true
		}
	}

	return false
}

// String representation of BondConfig, should be able to parse back into a
// BondConfig.
func (b BondConfig) String() string {
	parts := []string{}

	for _, i := range b.Interfaces {
		parts = append(parts, strconv.Itoa(i))
	}

	parts = append(parts, b.Mode)

	if !b.Fallback {
		parts = append(parts, b.LACP, "no-lacp-fallback")
	} else if b.LACP != defaultLACP {
		parts = append(parts, b.LACP)
	}

	if b.QinQ {
		parts = append(parts, "qinq")
	}

	if b.Name != "" && !strings.HasPrefix(b.Name, "mega_bond") {
		parts = append(parts, b.Name)
	}

	return strings.Join(parts, ",")
}

func (b BondConfigs) String() string {
	parts := []string{}

	for _, n := range b {
		parts = append(parts, n.String())
	}

	return strings.Join(parts, " ")
}

func (b BondConfigs) WriteConfig(w io.Writer) error {
	if len(b) > 0 {
		_, err := fmt.Fprintf(w, "vm config bonds %v\n", b)
		return err
	}

	return nil
}

var (
	bondModes = []string{"active-backup", "balance-slb", "balance-tcp"}
	lacpModes = []string{"active", "passive", "off"}

	defaultLACP = "active"
)

// ParseBondConfig processes the input specifying the interface indexes, bond
// mode, and LACP mode for interface bonds. This takes a bit of parsing, because
// the entry can be in a few forms:
//
//	 For the following, assume active LACP mode with LACP fallback
//		interfaces,mode
//		interfaces,mode,qinq
//		interfaces,mode,name
//		interfaces,mode,qinq,name
//
//	 For the following, assume LACP fallback
//		interfaces,mode,lacp
//		interfaces,mode,lacp,qinq
//		interfaces,mode,lacp,name
//		interfaces,mode,lacp,qinq,name
//
//		interfaces,mode,lacp,no-lacp-fallback
//		interfaces,mode,lacp,no-lacp-fallbak,qinq
//		interfaces,mode,lacp,no-lacp-fallback,name
//		interfaces,mode,lacp,no-lacp-fallback,qinq,name
func ParseBondConfig(spec string) (*BondConfig, error) {
	// example: 0,1,balance-tcp,active,no-lacp-fallback,qinq
	f := strings.Split(spec, ",")

	isBondMode := func(m string) bool {
		for _, mode := range bondModes {
			if strings.EqualFold(m, mode) {
				return true
			}
		}

		return false
	}

	isLACPMode := func(m string) bool {
		for _, mode := range lacpModes {
			if strings.EqualFold(m, mode) {
				return true
			}
		}

		return false
	}

	isNoFallback := func(f string) bool {
		return strings.EqualFold(f, "no-lacp-fallback")
	}

	isQinQ := func(q string) bool {
		return strings.EqualFold(q, "qinq")
	}

	var (
		ifaces  []string
		b, l, n string
		nf, q   bool
	)

	// must provide at least two interface indexes and a bond mode
	if len(f) < 3 {
		return nil, errors.New("malformed bondspec")
	}

	// find bond mode setting so interfaces to be bonded can be inferred
	for i, e := range f {
		if isBondMode(e) {
			ifaces = f[:i]
			b = e
			break
		}
	}

	if ifaces == nil {
		return nil, errors.New("malformed bondspec")
	}

	switch len(f) {
	case 4:
		if isLACPMode(f[3]) {
			l = f[3]
		} else if isQinQ(f[3]) {
			q = true
		} else { // assume bond name
			n = f[3]
		}
	case 5:
		if isQinQ(f[3]) {
			q, n = true, f[4]
		} else if isLACPMode(f[3]) && isNoFallback(f[4]) {
			l, nf = f[3], true
		} else if isLACPMode(f[3]) && isQinQ(f[4]) {
			l, q = f[3], true
		} else if isLACPMode(f[3]) {
			l, n = f[3], f[4]
		} else {
			return nil, errors.New("malformed bondspec")
		}
	case 6:
		if isLACPMode(f[3]) && isNoFallback(f[4]) && isQinQ(f[5]) {
			l, nf, q = f[3], true, true
		} else if isLACPMode(f[3]) && isNoFallback(f[4]) {
			l, nf, n = f[3], true, f[5]
		} else if isLACPMode(f[3]) && isQinQ(f[4]) {
			l, q, n = f[3], true, f[5]
		} else {
			return nil, errors.New("malformed bondspec")
		}
	case 7:
		if isLACPMode(f[3]) && isNoFallback(f[4]) && isQinQ(f[5]) {
			l, nf, q, n = f[3], true, true, f[6]
		} else {
			return nil, errors.New("malformed bondspec")
		}
	}

	log.Info(
		`got interfaces="%v", mode="%v", lacp="%v", fallback="%v", qinq="%v", name="%v"`,
		ifaces, b, l, !nf, q, n,
	)

	indexes := make([]int, len(ifaces))

	for i, iface := range ifaces {
		idx, err := strconv.Atoi(iface)
		if err != nil {
			return nil, fmt.Errorf("invalid interface index %s provided in bondspec", iface)
		}

		indexes[i] = idx
	}

	if l == "" {
		l = defaultLACP
	}

	return &BondConfig{
		Name:       n,
		Mode:       b,
		LACP:       l,
		Fallback:   !nf,
		QinQ:       q,
		Interfaces: indexes,
	}, nil
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
