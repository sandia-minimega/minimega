// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"fmt"
	log "minilog"
	"strings"
)

// upInterface activates an interface using the `ip` command. promisc controls
// whether the interface is brought up in promiscuous mode or not.
func upInterface(name string, promisc bool) error {
	log.Info("up interface: %v", name)

	args := []string{"ip", "link", "set", name, "up"}
	if promisc {
		args = append(args, "promisc", "on")
	}

	out, err := processWrapper(args...)
	if err != nil {
		return fmt.Errorf("up interface failed: %v: %v", err, out)
	}

	return nil
}

// downInterface deactivates an interface using the `ip` command.
func downInterface(name string) error {
	log.Info("down interface: %v", name)

	out, err := processWrapper("ip", "link", "set", name, "down")
	if err != nil {
		return fmt.Errorf("down interface failed: %v: %v", err, out)
	}

	return nil
}

// createTap creates a tuntap of the specified name using the `ip` command.
func createTap(name string) error {
	log.Info("creating tuntap: %v", name)

	out, err := processWrapper("ip", "tuntap", "add", "mode", "tap", name)
	if strings.Contains(out, "Device or resource busy") {
		return errAlreadyExists
	} else if err != nil {
		return fmt.Errorf("create tap failed: %v: %v", err, out)
	}

	return nil
}

// createVeth creates a veth of the specified name using the `ip` command.
func createVeth(tap, name, netnsname string) error {
	log.Debug("creating veth: %v on %v in netns %v", name, tap, netnsname)

	args := []string{
		"ip",
		"link",
		"add",
		tap,
		"type",
		"veth",
		"peer",
		name,
		"netns",
		netnsname,
	}

	out, err := processWrapper(args...)
	if err != nil {
		return fmt.Errorf("create veth failed: %v: %v", err, out)
	}

	return nil
}

// setMAC sets the MAC address for a container interface using the `ip` command.
func setMAC(netnsname, iface, mac string) error {
	log.Debug("setting MAC: %v %v %v", netnsname, iface, mac)

	args := []string{
		"ip",
		"netns",
		"exec",
		netnsname,
		"ip",
		"link",
		"set",
		"dev",
		iface,
		"address",
		mac,
	}

	out, err := processWrapper(args...)
	if err != nil {
		return fmt.Errorf("set MAC failed: %v: %v", err, out)
	}

	return nil
}

// DestroyTap destroys an `unmanaged` tap using the `ip` command. This can be
// used when cleaning up from a crash or when a tap is not connected to a
// bridges. See `Bridge.DestroyTap` for managed taps.
func DestroyTap(name string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return destroyTap(name)
}

// destroyTap destroys a tuntap device.
func destroyTap(name string) error {
	log.Info("destroying tuntap: %v", name)

	out, err := processWrapper("ip", "link", "del", name)
	if err != nil {
		return fmt.Errorf("destroy tap failed: %v: %v", err, out)
	}

	return nil
}
