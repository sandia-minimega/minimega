// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"fmt"
	"gonetflow"
	"gopacket/pcap"
	log "minilog"
	"strings"
	"sync"
	"time"
)

// Global lock for all bridge operations
var bridgeLock sync.Mutex

// Bridge stores state about an openvswitch bridge including the taps, tunnels,
// trunks, and netflow.
type Bridge struct {
	Name     string
	preExist bool

	mirror  string
	trunks  map[string]bool
	tunnels map[string]bool

	taps map[string]*Tap

	nf *gonetflow.Netflow

	// nameChan is a reference to the nameChan from the Bridges struct that
	// this Bridge was created on.
	nameChan chan string

	handle *pcap.Handle
}

// BridgeInfo is a summary of fields from a Bridge.
type BridgeInfo struct {
	Name     string
	PreExist bool
	VLANs    []int
	Mirror   string
	Trunks   []string
	Tunnels  []string
}

// Tap represents an interface that is attached to an openvswitch bridge.
type Tap struct {
	Name      string // Name of the tap
	Bridge    string // Bridge that the tap is connected to
	VLAN      int    // VLAN ID for the tap
	MAC       string // MAC address
	Host      bool   // Set when created as a host tap (and, thus, promiscuous)
	Container bool   // Set when created via CreateContainerTap
	Defunct   bool   // Set when Tap should be reaped

	IP4 string // Snooped IPv4 address
	IP6 string // Snooped IPv6 address
	Qos *qos   // Quality-of-service constraints

	stats []tapStat
}

type tapStat struct {
	t time.Time

	RxBytes int
	TxBytes int
}

// Destroy a bridge, removing all of the taps, etc. associated with it
func (b *Bridge) Destroy() error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.destroy()
}

func (b *Bridge) destroy() error {
	log.Info("destroying bridge: %v", b.Name)

	if b.handle != nil {
		b.handle.Close()
	}

	// first get all of the taps off of this bridge and destroy them
	for _, tap := range b.taps {
		if tap.Defunct {
			continue
		}

		log.Debug("destroying tap %v", tap.Name)
		if err := b.destroyTap(tap.Name); err != nil {
			log.Info("could not destroy tap: %v", err)
		}
	}

	for v := range b.trunks {
		if err := b.removeTrunk(v); err != nil {
			return err
		}
	}
	for v := range b.tunnels {
		if err := b.removeTunnel(v); err != nil {
			return err
		}
	}

	if b.mirror != "" {
		if err := b.destroyMirror(); err != nil {
			return err
		}
	}

	if b.nf != nil {
		if err := b.destroyNetflow(); err != nil {
			return err
		}
	}

	// make sure we actually reap the taps before we return
	if err := b.reapTaps(); err != nil {
		return err
	}

	// don't destroy the bridge if it existed before we started
	if b.preExist {
		return nil
	}

	return ovsDelBridge(b.Name)
}

// ReapTap should be called periodically to remove defunct taps.
func (b *Bridge) ReapTaps() error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.reapTaps()
}

// reapTaps deletes all defunct taps from a bridge using a single openvswitch
// del-port command. We do this to speed up the time it takes to remove
// openvswitch taps when a large number of taps are present on a bridge. See
// https://github.com/sandia-minimega/minimega/issues/296 for more discussion.
//
// A single del-port command in openvswitch is typically between 30-40
// characters, plus the 'ovs-vsctl' command. A command line buffer on a modern
// linux machine is something like 2MB (wow), so if we round the per-del-port
// up to 50 characters, we should be able to stack 40000 del-ports on a single
// command line. To that end we won't bother with setting a maximum number of
// taps to remove in a single operation. If we eventually get to 40k taps
// needing removal in a single pass of the reaper, then we have other problems.
//
// You can check yourself with `getconf ARG_MAX` or `xargs --show-limits`
func (b *Bridge) reapTaps() error {
	log.Debug("reaping taps on bridge: %v", b.Name)

	var args []string

	for _, tap := range b.taps {
		// build up the arg string directly for defunct taps
		if tap.Defunct {
			args = append(args, "--", "del-port", b.Name, tap.Name)
		}
	}

	if len(args) == 0 {
		return nil
	}

	log.Debug("reapTaps args: %v", strings.Join(args, " "))

	_, sErr, err := ovsCmdWrapper(args)
	if err != nil {
		return fmt.Errorf("reap taps failed: %v: %v", err, sErr)
	}

	// clean up state
	for _, tap := range b.taps {
		if tap.Defunct {
			delete(b.taps, tap.Name)
		}
	}

	return nil
}

// DestroyBridge deletes an `unmanaged` bridge. This can be used when cleaning
// up from a crash. See `Bride.Destroy` for managed bridges.
func DestroyBridge(name string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return ovsDelBridge(name)
}
