// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/gonetflow"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	"github.com/google/gopacket/pcap"
)

// Global lock for all bridge operations
var bridgeLock sync.Mutex

// Bridge stores state about an openvswitch bridge including the taps, tunnels,
// trunks, and netflow.
type Bridge struct {
	Name     string
	preExist bool

	// mirrors records the mirror tap names used by captures
	mirrors map[string]bool

	// captures records the "stop" flags that are set to non-zero values when
	// we want to stop a capture.
	captures map[int]capture

	trunks  map[string]bool
	tunnels map[string]bool

	taps map[string]*Tap

	nf *gonetflow.Netflow

	// nameChan is a reference to the nameChan from the Bridges struct that
	// this Bridge was created on.
	nameChan chan string

	handle *pcap.Handle

	// config values that have been set on this bridge
	config map[string]string

	// set to non-zero value by Bridge.destroy
	isdestroyed uint64
}

// BridgeInfo is a summary of fields from a Bridge.
type BridgeInfo struct {
	Name     string
	PreExist bool
	VLANs    []int
	Trunks   []string
	Tunnels  []string
	Mirrors  []string
	Config   map[string]string
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

	*qos // Quality-of-service constraints

	stats []tapStat
}

type capture struct {
	tap string

	// isstopped is set to non-zero when stopped
	isstopped *uint64

	// ack is closed when the goroutine doing the capture closes
	ack chan bool

	// pcap handle, needed so that we can close it in stopCapture
	handle *pcap.Handle
}

type tapStat struct {
	t time.Time

	RxBytes int
	TxBytes int
}

func (b *Bridge) destroy() error {
	log.Info("destroying bridge: %v", b.Name)

	if b.destroyed() {
		// bridge has already been destroyed
		return nil
	}

	b.setDestroyed()

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
	for v := range b.captures {
		b.stopCapture(v)
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
// https://github.com/sandia-minimega/minimega/v2/issues/296 for more discussion.
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

	if _, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("reap taps failed: %v", err)
	}

	// clean up state
	for _, tap := range b.taps {
		if tap.Defunct {
			delete(b.taps, tap.Name)
		}
	}

	return nil
}

func (b *Bridge) setDestroyed() {
	atomic.StoreUint64(&b.isdestroyed, 1)
}

func (b *Bridge) destroyed() bool {
	return atomic.LoadUint64(&b.isdestroyed) > 0
}

// DestroyBridge deletes an `unmanaged` bridge. This can be used when cleaning
// up from a crash. See `Bride.Destroy` for managed bridges.
func DestroyBridge(name string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return ovsDelBridge(name)
}
