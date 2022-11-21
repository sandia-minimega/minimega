// Copyright 2016-2022 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// CreateBondName will return the next created tap name from the name channel
func (b *Bridge) CreateBondName() string {
	return <-b.bondChan
}

func (b *Bridge) AddBond(name, mode, lacp string, fallback bool, interfaces map[string]int, vlan int) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("adding bond %v on bridge %v", name, b.Name)

	if _, ok := b.bonds[name]; ok {
		return fmt.Errorf("bond %v already exists on bridge %v", name, b.Name)
	}

	// ovs-vsctl add-bond <bridge name> <bond name> <list of interfaces>
	args := []string{"add-bond", b.Name, name}

	for iface := range interfaces {
		args = append(args, iface)

		// need to remove the taps from ovs first before we can bond
		if err := ovsDelPort(b.Name, iface); err != nil {
			return fmt.Errorf("failed to delete tap %v from ovs with error: %v", iface, err)
		}
	}

	args = append(args, "--", "set", "port", name)
	args = append(args, fmt.Sprintf("tag=%v", vlan))

	// https://www.man7.org/linux/man-pages/man5/ovs-vswitchd.conf.db.5.html#Port_TABLE
	// https://access.redhat.com/documentation/en-us/red_hat_openstack_platform/13/html/advanced_overcloud_customization/overcloud-network-interface-bonding#open-vswitch-bonding-options
	switch mode {
	case "active-backup", "balance-slb", "balance-tcp":
		if mode == "balance-tcp" && lacp == "off" {
			return fmt.Errorf("LACP mode must be set to active or passive for balance-tcp bond mode")
		}

		args = append(args, fmt.Sprintf("lacp=%s", lacp))

		if lacp != "off" && fallback {
			args = append(args, "other_config:lacp-fallback-ab=true")
		}

		args = append(args, fmt.Sprintf("bond_mode=%s", mode))
	default:
		return fmt.Errorf("unsupported bond mode provided: %s", mode)
	}

	if _, err := ovsCmdWrapper(args); err != nil {
		// add taps back to ovs since bond wasn't created
		for iface, vid := range interfaces {
			ovsAddPort(b.Name, iface, vid, false)
		}

		return fmt.Errorf("add bond failed: %v", err)
	}

	bonded := make(map[string]int)

	// can't avoid looping over interfaces twice...
	// at least it won't ever be a big slice
	for iface, vid := range interfaces {
		tap, ok := b.taps[iface]
		if ok {
			tap.Bond = name
			b.taps[iface] = tap
		} else {
			// not really ever expected to happen...
			log.Error("well this is awkward... tap %v isn't known on bridge %v", iface, b.Name)
		}

		bonded[iface] = vid
	}

	b.bonds[name] = bonded
	return nil
}

func (b *Bridge) DeleteBond(name string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.deleteBond(name)
}

func (b *Bridge) deleteBond(name string) error {
	log.Info("deleting bond %v on bridge %v", name, b.Name)

	if _, ok := b.bonds[name]; !ok {
		return fmt.Errorf("bond %v does not exist on bridge %v", name, b.Name)
	}

	if err := ovsDelPort(b.Name, name); err != nil {
		return fmt.Errorf("failed to delete bond %v from ovs with error: %v", name, err)
	}

	for iface, vlan := range b.bonds[name] {
		if err := ovsAddPort(b.Name, iface, vlan, false); err != nil {
			return fmt.Errorf("failed to recreate tap %v on bridge %v: %v", iface, b.Name, err)
		}

		tap, ok := b.taps[iface]
		if ok {
			tap.Bond = ""
			b.taps[iface] = tap
		} else {
			// not really ever expected to happen...
			log.Error("well this is awkward... tap %v isn't known on bridge %v", iface, b.Name)
		}
	}

	delete(b.bonds, name)
	return nil
}
