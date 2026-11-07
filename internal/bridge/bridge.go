// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"
	"sort"
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

	// bonds tracks bonded interfaces added to the bridge, along with the VLAN the
	// interface originally belonged to. The bond name is the key to the first map
	// and the interface name is the key to the second map.
	bonds map[string]map[string]int

	// captures records the "stop" flags that are set to non-zero values when
	// we want to stop a capture.
	captures map[int]capture

	// captureID is a monotonically increasing counter used to assign unique
	// IDs to captures. IDs must never be reused, even after captures are
	// stopped and removed from the captures map or the bridge is recreated.
	captureID *int

	trunks  map[string]bool
	tunnels map[string]bool

	taps map[string]*Tap

	nf *gonetflow.Netflow

	// tapChan is a reference to the tapChan from the Bridges struct that
	// this Bridge was created on.
	tapChan chan string

	// bondChan is a reference to the bondChan from the Bridges struct that
	// this Bridge was created on.
	bondChan chan string

	handle *pcap.Handle

	// config values that have been set on this bridge
	config map[string]string

	// set to non-zero value by Bridge.destroy
	isdestroyed uint64

	// set when Bridge.destroy deleted the OVS bridge
	ovsDeleted bool

	// reaper removes the OVS ports and devices of defunct taps
	reaper tapReaper
}

// tapReaper removes the OVS ports and the devices of defunct taps for
// Bridge.reapTaps.
type tapReaper interface {
	// delPorts removes the ports from the bridge in one transaction.
	delPorts(bridge string, ports []string) error

	// delLinks destroys the devices and returns the names of the devices
	// that no longer exist.
	delLinks(names []string) ([]string, error)

	// delBridge deletes the OVS bridge and all of its ports.
	delBridge(bridge string) error
}

// systemReaper is the tapReaper that runs `ovs-vsctl` and uses netlink.
type systemReaper struct{}

func (systemReaper) delPorts(bridge string, ports []string) error {
	args := delPortArgs(bridge, ports)

	log.Debug("reapTaps args: %v", strings.Join(args, " "))

	_, err := ovsCmdWrapper(args)
	return err
}

// delPortArgs returns the ovs-vsctl arguments that remove the ports from the
// bridge in one transaction. --if-exists lets a retry succeed when some of the
// ports are already gone.
func delPortArgs(bridge string, ports []string) []string {
	var args []string

	for _, port := range ports {
		args = append(args, "--", "--if-exists", "del-port", bridge, port)
	}

	return args
}

func (systemReaper) delLinks(names []string) ([]string, error) {
	return destroyTaps(names)
}

func (systemReaper) delBridge(bridge string) error {
	return ovsDelBridge(bridge)
}

// BridgeInfo is a summary of fields from a Bridge.
type BridgeInfo struct {
	Name     string
	PreExist bool
	VLANs    []int
	Trunks   []string
	Tunnels  []string
	Mirrors  []string
	Bonds    map[string][]string
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
	Bond      string // Bond that the tap belongs to
	Defunct   bool   // Set when Tap should be reaped

	portReaped bool // Set when reapTaps removed the OVS port
	deleteLink bool // Set when reapTaps must destroy the device

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
		// a destroy that failed can leave taps and the OVS bridge to clean up
		return b.finishDestroy()
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

	return b.finishDestroy()
}

// finishDestroy reaps the taps and deletes the OVS bridge. It returns nil when
// nothing is left to clean up, so it can be called again after a failure.
func (b *Bridge) finishDestroy() error {
	// del-br still removes the ports if the reap fails
	err := b.reapTaps()

	// don't destroy the bridge if it existed before we started
	if b.preExist {
		return err
	}

	if !b.ovsDeleted {
		if err2 := b.reaper.delBridge(b.Name); err2 != nil {
			return errors.Join(err, err2)
		}

		b.ovsDeleted = true

		for _, tap := range b.taps {
			tap.portReaped = true
		}

		b.forgetReapedTaps()
	}

	if len(b.taps) == 0 {
		return nil
	}

	return err
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
//
// After the del-port command, reapTaps destroys the devices of the defunct
// taps in one netlink operation (see destroyTaps). A tap stays on the bridge
// until both its port and its device are removed.
func (b *Bridge) reapTaps() error {
	log.Debug("reaping taps on bridge: %v", b.Name)

	var ports, links []string

	for _, tap := range b.taps {
		if !tap.Defunct {
			continue
		}

		if tap.Bond == "" && !tap.portReaped {
			ports = append(ports, tap.Name)
		}
		if tap.deleteLink {
			links = append(links, tap.Name)
		}
	}

	if len(ports) == 0 && len(links) == 0 {
		return nil
	}

	sort.Strings(ports)
	sort.Strings(links)

	var errs []error

	if len(ports) > 0 {
		if err := b.reaper.delPorts(b.Name, ports); err != nil {
			errs = append(errs, fmt.Errorf("reap taps failed: %v", err))
		} else {
			for _, name := range ports {
				b.taps[name].portReaped = true
			}
		}
	}

	if len(links) > 0 {
		gone, err := b.reaper.delLinks(links)
		for _, name := range gone {
			if tap, ok := b.taps[name]; ok {
				tap.deleteLink = false
			}
		}

		if err != nil {
			errs = append(errs, fmt.Errorf("reap tap devices failed: %w", err))
		}
	}

	b.forgetReapedTaps()

	return errors.Join(errs...)
}

// forgetReapedTaps removes the defunct taps whose port and device are gone.
func (b *Bridge) forgetReapedTaps() {
	for name, tap := range b.taps {
		if tap.Defunct && (tap.Bond != "" || tap.portReaped) && !tap.deleteLink {
			delete(b.taps, name)
		}
	}
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
