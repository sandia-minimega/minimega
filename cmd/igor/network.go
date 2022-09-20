package main

import (
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	networkSetFuncs   map[string]func([]string, int) error
	networkClearFuncs map[string]func([]string, int) error
)

// Configure the given nodes into the specified 802.1ad outer VLAN
func networkSet(nodes []string, vlan int) error {
	if igor.Network == "" {
		// they don't want to do vlan segmentation
		log.Debug("not doing vlan segmentation")
		return nil
	}

	f, ok := networkSetFuncs[igor.Network]
	if !ok {
		log.Fatal("no such network mode: %v", igor.Network)
	}
	return f(nodes, vlan)
}

// Clear any 802.1ad configuration on the given nodes
func networkClear(nodes []string, vlan int) error {
	if igor.Network == "" {
		// they don't want to do vlan segmentation
		log.Debug("not doing vlan segmentation")
		return nil
	}

	f, ok := networkClearFuncs[igor.Network]
	if !ok {
		log.Fatal("no such network mode: %v", igor.Network)
	}
	return f(nodes, vlan)
}
