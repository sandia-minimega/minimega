// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	"github.com/google/gopacket/pcap"
)

// Bridges manages a collection of `Bridge` structs.
type Bridges struct {
	Default string // Default bridge name when one isn't specified

	nameChan chan string
	bridges  map[string]*Bridge
}

// openflow filters to redirect arp and icmp6 traffic to the local tap
var snoopFilters = []string{
	"dl_type=0x0806,actions=local,normal",
	"dl_type=0x86dd,nw_proto=58,icmp_type=135,actions=local,normal",
}

// snoopBPF filters for ARP and Neighbor Solicitation (NDP)
const snoopBPF = "(arp or (icmp6 and ip6[40] == 135))"

// NewBridges creates a new Bridges using d as the default bridge name and f as
// the format string for the tap names (e.g. "mega_tap%v").
func NewBridges(d, f string) *Bridges {
	nameChan := make(chan string)

	// Start a goroutine to generate tap names for us
	go func() {
		defer close(nameChan)

		for tapCount := 0; ; tapCount++ {
			tapName := fmt.Sprintf(f, tapCount)
			fpath := filepath.Join("/sys/class/net", tapName)

			if _, err := os.Stat(fpath); os.IsNotExist(err) {
				nameChan <- tapName
			} else if err != nil {
				log.Fatal("unable to stat file -- %v %v", fpath, err)
			}

			log.Debug("tapCount: %v", tapCount)
		}
	}()

	b := &Bridges{
		Default:  d,
		nameChan: nameChan,
		bridges:  map[string]*Bridge{},
	}

	// Start a goroutine to collect bandwidth stats every 5 seconds
	go func() {
		for {
			time.Sleep(5 * time.Second)

			b.updateBandwidthStats()
		}
	}()

	return b
}

// newBridge creates a new bridge with ovs, assumes that bridgeLock is held.
func (b Bridges) newBridge(name string) error {
	log.Info("creating new bridge: %v", name)

	br := &Bridge{
		Name:     name,
		taps:     make(map[string]*Tap),
		trunks:   make(map[string]bool),
		tunnels:  make(map[string]bool),
		mirrors:  make(map[string]bool),
		captures: make(map[int]capture),
		nameChan: b.nameChan,
		config:   make(map[string]string),
	}

	// Create the bridge
	created, err := ovsAddBridge(br.Name)
	if err != nil {
		return err
	}

	br.preExist = !created

	// Bring the interface up, start MAC <-> IP learner
	if err = upInterface(br.Name, false); err != nil {
		goto cleanup
	}

	for _, filter := range snoopFilters {
		if err = br.addOpenflow(filter); err != nil {
			goto cleanup
		}
	}

	if br.handle, err = pcap.OpenLive(br.Name, 1600, true, time.Second); err != nil {
		goto cleanup
	}

	if err = br.handle.SetBPFFilter(snoopBPF); err != nil {
		goto cleanup
	}

	go br.snooper()

	// No errors... bridge ready for use
	b.bridges[br.Name] = br
	return nil

cleanup:
	if err != nil {
		log.Errorln(err)
	}

	if br.handle != nil {
		br.handle.Close()
	}

	// Try to delete the bridge, if we created it
	if created {
		if err := ovsDelBridge(br.Name); err != nil {
			// Welp, we're boned
			log.Error("zombie bridge -- %v %v", br.Name, err)
		}
	}

	return err
}

// Names returns a list of all the managed bridge names.
func (b Bridges) Names() []string {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	res := []string{}
	for k := range b.bridges {
		res = append(res, k)
	}

	return res
}

// Get a bridge by name. If one doesn't exist, it will be created.
func (b Bridges) Get(name string) (*Bridge, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	if name == "" {
		name = b.Default
	}

	// Test if the bridge already exists
	if v, ok := b.bridges[name]; ok {
		return v, nil
	}

	// Doesn't exist, create it
	if err := b.newBridge(name); err != nil {
		return nil, err
	}

	return b.bridges[name], nil
}

// HostTaps returns a list of taps that are marked as host taps.
func (b Bridges) HostTaps() []Tap {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	res := []Tap{}

	for _, br := range b.bridges {
		for _, tap := range br.taps {
			if tap.Host && !tap.Defunct {
				res = append(res, *tap)
			}
		}
	}

	return res
}

// Info collects `BridgeInfo` for all managed bridges.
func (b Bridges) Info() []BridgeInfo {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	res := []BridgeInfo{}

	for _, br := range b.bridges {
		info := BridgeInfo{
			Name:     br.Name,
			PreExist: br.preExist,
			Config:   make(map[string]string),
		}

		// Populate trunks
		for k := range br.trunks {
			info.Trunks = append(info.Trunks, k)
		}
		sort.Strings(info.Trunks)

		// Populate tunnels
		for k := range br.tunnels {
			info.Tunnels = append(info.Tunnels, k)
		}
		sort.Strings(info.Tunnels)

		// Populate mirrors
		for k := range br.mirrors {
			info.Mirrors = append(info.Mirrors, k)
		}
		sort.Strings(info.Mirrors)

		// Populate VLANs
		vlans := map[int]bool{}
		for _, tap := range br.taps {
			if !tap.Defunct {
				vlans[tap.VLAN] = true
			}
		}

		for k, _ := range vlans {
			info.VLANs = append(info.VLANs, k)
		}
		sort.Ints(info.VLANs)

		// Populate config
		for k, v := range br.config {
			info.Config[k] = v
		}

		res = append(res, info)
	}

	return res
}

// Destroy calls `Bridge.Destroy` on each bridge, returning the first error.
func (b Bridges) Destroy() error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	for k, br := range b.bridges {
		if err := br.destroy(); err != nil {
			return err
		}

		delete(b.bridges, k)
	}

	return nil
}

// DestroyBridge destroys a bridge by name, removing all of the taps, etc.
// associated with it.
func (b Bridges) DestroyBridge(name string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	br, ok := b.bridges[name]
	if !ok {
		return fmt.Errorf("bridge not found: %v", name)
	}

	if err := br.destroy(); err != nil {
		return err
	}

	delete(b.bridges, name)
	return nil
}

// ReapTaps calls `Bridge.ReapTaps` on each bridge, returning the first error.
func (b Bridges) ReapTaps() error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	for _, br := range b.bridges {
		if err := br.reapTaps(); err != nil {
			return err
		}
	}

	return nil
}

// FindTap finds a non-defunct Tap with the specified name. This is
// non-deterministic if there are multiple taps with the same name.
func (b Bridges) FindTap(t string) (Tap, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Debug("searching for tap %v", t)

	for _, br := range b.bridges {
		for _, tap := range br.taps {
			if tap.Name == t && !tap.Defunct {
				log.Debug("found tap %v on bridge %v", t, br.Name)
				return *tap, nil
			}
		}
	}

	return Tap{}, errors.New("unknown tap")
}
