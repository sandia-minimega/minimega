// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"
	log "minilog"
)

// TunnelType is used to specify the type of tunnel for `AddTunnel`.
type TunnelType int

const (
	TunnelVXLAN TunnelType = iota
	TunnelGRE
)

func (t TunnelType) String() string {
	switch t {
	case TunnelVXLAN:
		return "vxlan"
	case TunnelGRE:
		return "gre"
	}

	return "invalid"
}

// AddTunnel adds a new vxlan or GRE tunnel to a bridge.
func (b *Bridge) AddTunnel(typ TunnelType, remoteIP, key string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("adding tunnel on bridge %v: %v %v", b.Name, typ, remoteIP)

	tap := <-b.nameChan

	args := []string{
		"add-port",
		b.Name,
		tap,
		"--",
		"set",
		"interface",
		tap,
		fmt.Sprintf("type=%v", typ),
		fmt.Sprintf("options:remote_ip=%v", remoteIP),
	}
	if key != "" {
		args = append(args, fmt.Sprintf("options:key=%v", key))
	}
	if _, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("add tunnel failed: %v", err)
	}

	b.tunnels[tap] = true

	return nil
}

// RemoveTunnel removes a tunnel from the bridge.
func (b *Bridge) RemoveTunnel(iface string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.removeTunnel(iface)
}

func (b *Bridge) removeTunnel(iface string) error {
	log.Info("removing tunnel on bridge %v: %v", b.Name, iface)

	if !b.tunnels[iface] {
		return fmt.Errorf("unknown tunnel: %v", iface)
	}

	err := ovsDelPort(b.Name, iface)
	if err == nil {
		delete(b.tunnels, iface)
	}

	// TODO: Need to destroy the interface?

	return err
}
