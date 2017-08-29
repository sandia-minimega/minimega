package main

import (
	log "minilog"
)

var (
	networkSetFuncs   map[string]func([]string, int) error
	networkClearFuncs map[string]func([]string) error
)

func networkSet(nodes []string, vlan int) error {
	if igorConfig.Network == "" {
		// they don't want to do vlan segmentation
		log.Debug("not doing vlan segmentation")
		return nil
	}

	f, ok := networkSetFuncs[igorConfig.Network]
	if !ok {
		log.Fatal("no such network mode: %v", igorConfig.Network)
	}
	return f(nodes, vlan)
}

func networkClear(nodes []string) error {
	if igorConfig.Network == "" {
		// they don't want to do vlan segmentation
		log.Debug("not doing vlan segmentation")
		return nil
	}

	f, ok := networkClearFuncs[igorConfig.Network]
	if !ok {
		log.Fatal("no such network mode: %v", igorConfig.Network)
	}
	return f(nodes)
}
