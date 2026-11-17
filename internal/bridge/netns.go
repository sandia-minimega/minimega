// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux

package bridge

import (
	"fmt"
	"os"
	"runtime"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/sys/unix"
)

// netnsSelf is the network namespace of the calling thread.
const netnsSelf = "/proc/thread-self/ns/net"

// Tests replace these.
var (
	netnsUnshare = func() error { return unix.Unshare(unix.CLONE_NEWNET) }
	netnsSetns   = func(fd int) error { return unix.Setns(fd, unix.CLONE_NEWNET) }

	netnsAddVeth   = addNetnsVeth
	netnsSetupHost = setupNetnsHost
	netnsSetupTap  = setupNetnsTap
	netnsDelLink   = DeleteLink
)

// netnsAddTap adds the host end to the bridge. The caller holds bridgeLock.
// Tests replace it.
var netnsAddTap = func(b *Bridge, tap, mac string, vlan int) error {
	return b.addTap(tap, mac, vlan, false)
}

// Netns is a network namespace that minimega holds open as a file
// descriptor, with no name in /run/netns. The namespace exists until Close
// and until every process in it exits.
type Netns struct {
	f  *os.File
	fd int
}

// NewNetns creates a network namespace.
func NewNetns() (*Netns, error) {
	var ns *Netns

	err := onNetnsThread(func() (bool, error) {
		orig, err := os.Open(netnsSelf)
		if err != nil {
			return true, fmt.Errorf("create netns: %w", err)
		}
		defer orig.Close()

		if err := netnsUnshare(); err != nil {
			return true, fmt.Errorf("create netns: %w", err)
		}

		f, openErr := os.Open(netnsSelf)

		if err := netnsSetns(int(orig.Fd())); err != nil {
			if f != nil {
				f.Close()
			}
			return false, fmt.Errorf("create netns: restore thread netns: %w", err)
		}
		if openErr != nil {
			return true, fmt.Errorf("create netns: %w", openErr)
		}

		ns = &Netns{f: f, fd: int(f.Fd())}
		return true, nil
	})

	return ns, err
}

// Path returns a path that opens the namespace while it is open in minimega,
// for example for `jailer --netns`.
func (ns *Netns) Path() string {
	if ns == nil || ns.f == nil {
		return ""
	}

	return fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), ns.fd)
}

// Close releases the namespace. The kernel removes it, and the links in it,
// when no process is left in it.
func (ns *Netns) Close() error {
	if ns == nil || ns.f == nil {
		return nil
	}

	err := ns.f.Close()
	ns.f = nil
	return err
}

// Do runs fn on an OS thread in the namespace. Goroutines that fn starts run
// outside of it.
func (ns *Netns) Do(fn func() error) error {
	if ns == nil || ns.f == nil {
		return fmt.Errorf("netns is closed")
	}

	return onNetnsThread(func() (bool, error) {
		orig, err := os.Open(netnsSelf)
		if err != nil {
			return true, fmt.Errorf("enter netns: %w", err)
		}
		defer orig.Close()

		if err := netnsSetns(ns.fd); err != nil {
			return true, fmt.Errorf("enter netns: %w", err)
		}

		ferr := fn()

		if err := netnsSetns(int(orig.Fd())); err != nil {
			return false, fmt.Errorf("leave netns: %w", err)
		}

		return true, ferr
	})
}

// onNetnsThread runs fn on a new locked OS thread. When fn returns false, the
// thread stays locked and Go discards it with the goroutine, because the
// thread is in another namespace.
func onNetnsThread(fn func() (bool, error)) error {
	ch := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		restored, err := fn()
		if restored {
			runtime.UnlockOSThread()
		}
		ch <- err
	}()

	return <-ch
}

// DeleteLink deletes the named link in the network namespace of minimega.
// Deleting one end of a veth pair deletes both ends.
func DeleteLink(name string) error {
	c, err := nlDial()
	if err != nil {
		return err
	}
	defer c.Close()

	return c.linkDel(name)
}

// CreateNetnsTap connects a tap in the network namespace ns to the bridge. It
// creates a veth pair with the host end on the bridge and the peer vm<index>
// in ns, then a tap fc<index> in ns that uid owns, and redirects all traffic
// between vm<index> and fc<index> with tc. mac is the guest MAC address, used
// to snoop the guest IP addresses. It returns the host end name and its MTU.
// Only the OVS port and the tap table update hold the bridge lock. With
// OVSBatching, the OVS port add waits on the batcher without the lock.
func (b *Bridge) CreateNetnsTap(ns *Netns, mac string, vlan, index, uid int) (string, int, error) {
	tap, mtu, err := b.CreateNetnsLinks(ns, mac, vlan, index, uid)
	if err != nil {
		return "", 0, err
	}

	if err := b.AddNetnsTap(tap, mac, vlan); err != nil {
		return "", 0, err
	}

	return tap, mtu, nil
}

// CreateNetnsLinks is the netlink part of CreateNetnsTap: the veth pair, the
// tap, and the tc redirects. It returns the host end name and its MTU.
// AddNetnsTap then adds the host end to the bridge. On failure, it deletes the
// veth pair.
func (b *Bridge) CreateNetnsLinks(ns *Netns, mac string, vlan, index, uid int) (string, int, error) {
	tap := <-b.tapChan
	peer := fmt.Sprintf("vm%d", index)

	log.Info("creating netns tap on bridge %v: %v %v %v %v", b.Name, tap, mac, vlan, index)

	if err := netnsAddVeth(tap, peer, ns); err != nil {
		return "", 0, err
	}

	mtu, err := netnsSetupHost(tap)
	if err == nil {
		err = netnsSetupTap(ns, index, uid, mtu)
	}
	if err != nil {
		deleteNetnsLink(tap)
		return "", 0, err
	}

	return tap, mtu, nil
}

// AddNetnsTap adds the host end tap from CreateNetnsLinks to the bridge. On
// failure, it deletes the veth pair.
func (b *Bridge) AddNetnsTap(tap, mac string, vlan int) error {
	var err error
	if OVSBatching {
		err = b.addNetnsTapBatched(tap, mac, vlan)
	} else {
		bridgeLock.Lock()
		err = netnsAddTap(b, tap, mac, vlan)
		bridgeLock.Unlock()
	}

	if err != nil {
		deleteNetnsLink(tap)
	}

	return err
}

func deleteNetnsLink(tap string) {
	if err := netnsDelLink(tap); err != nil {
		log.Error("zombie tap -- %v %v", tap, err)
	}
}

// DestroyNetnsTap marks a tap that CreateNetnsTap created as defunct and
// deletes its veth pair. ReapTaps removes it from the bridge.
func (b *Bridge) DestroyNetnsTap(tap string) error {
	log.Info("destroying netns tap on bridge: %v %v", b.Name, tap)

	bridgeLock.Lock()
	t, ok := b.taps[tap]
	if ok {
		t.Defunct = true
	}
	bridgeLock.Unlock()

	if !ok {
		return fmt.Errorf("unknown tap: %v", tap)
	}

	return netnsDelLink(tap)
}

// addNetnsVeth creates the veth pair tap and peer, with peer in ns.
func addNetnsVeth(tap, peer string, ns *Netns) error {
	c, err := nlDial()
	if err != nil {
		return err
	}
	defer c.Close()

	return c.linkAddVethPeerNetns(tap, peer, ns)
}

// setupNetnsHost sets the host end tap up and returns its MTU.
func setupNetnsHost(tap string) (int, error) {
	c, err := nlDial()
	if err != nil {
		return 0, err
	}
	defer c.Close()

	if err := c.linkSet(tap, 0, true); err != nil {
		return 0, err
	}

	l, err := c.linkGet(tap)
	if err != nil {
		return 0, err
	}

	return l.MTU, nil
}

// setupNetnsTap creates tap fc<index> in ns, sets it and vm<index> up with
// mtu, and redirects traffic both ways between them.
func setupNetnsTap(ns *Netns, index, uid, mtu int) error {
	veth := fmt.Sprintf("vm%d", index)
	tap := fmt.Sprintf("fc%d", index)

	return ns.Do(func() error {
		if err := createTunTap(tap, uid); err != nil {
			return err
		}

		c, err := nlDial()
		if err != nil {
			return err
		}
		defer c.Close()

		var idx [2]int32
		for i, name := range []string{veth, tap} {
			if err := c.linkSet(name, mtu, true); err != nil {
				return err
			}
			l, err := c.linkGet(name)
			if err != nil {
				return err
			}
			idx[i] = l.Index
		}

		for _, p := range [][2]int32{{idx[0], idx[1]}, {idx[1], idx[0]}} {
			if err := c.qdiscAddIngress(p[0]); err != nil {
				return err
			}
			if err := c.filterAddRedirect(p[0], p[1]); err != nil {
				return err
			}
		}

		return nil
	})
}

// addNetnsTapBatched adds the host end to the bridge through the OVS batcher.
// It holds bridgeLock only to reap taps and to update the tap table.
func (b *Bridge) addNetnsTapBatched(tap, mac string, vlan int) error {
	log.Info("adding tap on bridge: %v %v %v %v %v", b.Name, tap, mac, vlan, false)

	bridgeLock.Lock()
	b.reapTaps()
	_, exists := b.taps[tap]
	bridgeLock.Unlock()

	if exists {
		return fmt.Errorf("tap already on bridge")
	}

	err := ovsAddPortBatched(b.Name, tap, vlan, false)
	if err == errAlreadyExists {
		log.Info("tap %v is already on bridge, adding again", tap)
		if err = ovsDelPort(b.Name, tap); err == nil {
			err = ovsAddPortBatched(b.Name, tap, vlan, false)
		}
	}
	if err != nil {
		return err
	}

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	b.taps[tap] = &Tap{
		Name:   tap,
		Bridge: b.Name,
		VLAN:   vlan,
		MAC:    mac,
	}

	return nil
}
