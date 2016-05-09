// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"errors"
	"fmt"
	log "minilog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Bridges manages a collection of `Bridge` structs.
type Bridges struct {
	Default string // Default bridge name when one isn't specified

	nameChan chan string
	bridges  map[string]*Bridge
}

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
		nameChan: b.nameChan,
	}

	// Create the bridge
	created, err := ovsAddBridge(br.Name)
	if err != nil {
		return err
	}

	br.preExist = !created

	// Bring the interface up
	if err := upInterface(br.Name, false); err != nil {
		if err := ovsDelBridge(br.Name); err != nil {
			// Welp, we're boned
			log.Error("zombie bridge -- %v %v", br.Name, err)
		}

		return err
	}

	br.startIML()

	b.bridges[br.Name] = br

	return nil
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
			Mirror:   br.mirror,
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
				log.Info("found tap %v on bridge %v", t, br.Name)
				return *tap, nil
			}
		}
	}

	return Tap{}, errors.New("unknown tap")
}
