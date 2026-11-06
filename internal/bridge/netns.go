// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Tests replace these.
var (
	netnsRun    = processWrapper
	netnsDir    = "/var/run/netns"
	netnsSysNet = "/sys/class/net"
)

// netnsAddTap adds the host end to the bridge. The caller holds bridgeLock.
// Tests replace it.
var netnsAddTap = func(b *Bridge, tap, mac string, vlan int) error {
	return b.addTap(tap, mac, vlan, false)
}

// CreateNetns creates the named network namespace. It deletes a leftover
// namespace with the same name first.
func CreateNetns(ns string) error {
	if _, err := os.Stat(filepath.Join(netnsDir, ns)); err == nil {
		if err := DestroyNetns(ns); err != nil {
			return err
		}
	}

	if out, err := netnsRun("ip", "netns", "add", ns); err != nil {
		return fmt.Errorf("create netns %v failed: %v: %v", ns, err, out)
	}

	return nil
}

// DestroyNetns deletes the named network namespace. A missing namespace is
// not an error.
func DestroyNetns(ns string) error {
	out, err := netnsRun("ip", "netns", "del", ns)
	if err != nil && !strings.Contains(out, "No such file") {
		return fmt.Errorf("destroy netns %v failed: %v: %v", ns, err, out)
	}

	return nil
}

// CreateNetnsTap connects a tap in the network namespace ns to the bridge. It
// creates a veth pair with the host end on the bridge and the peer vm<index>
// in ns, then a tap fc<index> in ns that uid owns, and redirects all traffic
// between vm<index> and fc<index> with tc. mac is the guest MAC address, used
// to snoop the guest IP addresses. It returns the host end name and its MTU.
// Only the OVS port and the tap table update hold the bridge lock.
func (b *Bridge) CreateNetnsTap(ns, mac string, vlan, index, uid int) (string, int, error) {
	tap := <-b.tapChan
	peer := fmt.Sprintf("vm%d", index)

	log.Info("creating netns tap on bridge %v: %v %v %v %v %v", b.Name, tap, ns, mac, vlan, index)

	if out, err := netnsRun("ip", "link", "add", tap, "type", "veth", "peer", "name", peer, "netns", ns); err != nil {
		return "", 0, fmt.Errorf("create veth failed: %v: %v", err, out)
	}

	mtu, err := b.setupNetnsTap(tap, ns, mac, vlan, index, uid)
	if err != nil {
		if out, err := netnsRun("ip", "link", "del", tap); err != nil {
			log.Error("zombie tap -- %v %v: %v", tap, err, out)
		}

		return "", 0, err
	}

	return tap, mtu, nil
}

func (b *Bridge) setupNetnsTap(tap, ns, mac string, vlan, index, uid int) (int, error) {
	if out, err := netnsRun("ip", "link", "set", tap, "up"); err != nil {
		return 0, fmt.Errorf("up interface failed: %v: %v", err, out)
	}

	data, err := os.ReadFile(filepath.Join(netnsSysNet, tap, "mtu"))
	if err != nil {
		return 0, fmt.Errorf("read MTU of %v: %v", tap, err)
	}
	mtu, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("read MTU of %v: %v", tap, err)
	}

	for _, args := range netnsTapCommands(ns, index, uid, mtu) {
		if out, err := netnsRun(args...); err != nil {
			return 0, fmt.Errorf("netns tap setup failed: %v: %v: %v", strings.Join(args, " "), err, out)
		}
	}

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return mtu, netnsAddTap(b, tap, mac, vlan)
}

// netnsTapCommands returns the commands that create tap fc<index> in ns and
// redirect traffic both ways between it and vm<index>.
func netnsTapCommands(ns string, index, uid, mtu int) [][]string {
	veth := fmt.Sprintf("vm%d", index)
	tap := fmt.Sprintf("fc%d", index)
	id := strconv.Itoa(uid)
	m := strconv.Itoa(mtu)

	cmds := [][]string{
		{"ip", "-n", ns, "tuntap", "add", "dev", tap, "mode", "tap", "user", id, "group", id},
		{"ip", "-n", ns, "link", "set", "dev", veth, "mtu", m},
		{"ip", "-n", ns, "link", "set", "dev", tap, "mtu", m},
		{"ip", "-n", ns, "link", "set", "dev", veth, "up"},
		{"ip", "-n", ns, "link", "set", "dev", tap, "up"},
	}

	for _, p := range [][2]string{{veth, tap}, {tap, veth}} {
		cmds = append(cmds,
			[]string{"tc", "-n", ns, "qdisc", "add", "dev", p[0], "ingress"},
			[]string{"tc", "-n", ns, "filter", "add", "dev", p[0], "parent", "ffff:", "matchall", "action", "mirred", "egress", "redirect", "dev", p[1]},
		)
	}

	return cmds
}
