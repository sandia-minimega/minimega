// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"errors"
	"fmt"
	log "minilog"
	"os/exec"
	"strings"
)

var (
	errAlreadyExists = errors.New("already exists")
	errNoSuchPort    = errors.New("no such port")
)

// ovsAddBridge creates a new openvswitch bridge. Returns whether the bridge
// was created or not, or any error that occurred.
func ovsAddBridge(name string) (bool, error) {
	args := []string{
		"add-br",
		name,
	}

	// Linux limits interfaces to IFNAMSIZ bytes which is currently 16,
	// including the null byte. We won't return an error as this limit may not
	// affect the user but we should at least warn them that openvswitch may
	// fail unexectedly.
	if len(name) > 15 {
		log.Warn("bridge name is longer than 15 characters.. dragons ahead")
	}

	if _, err := ovsCmdWrapper(args); err == errAlreadyExists {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("add bridge failed: %v", err)
	}

	return true, nil
}

// ovsDelBridge deletes a openvswitch bridge.
func ovsDelBridge(name string) error {
	args := []string{
		"del-br",
		name,
	}

	if _, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("delete bridge failed: %v", err)
	}

	return nil
}

// ovsAddPort adds a port to an openvswitch bridge. If the vlan is 0, it will
// not be vlan-tagged.
func ovsAddPort(bridge, tap string, vlan int, host bool) error {
	args := []string{
		"add-port",
		bridge,
		tap,
	}

	// see note in ovsAddBridge.
	if len(tap) > 15 {
		log.Warn("tap name is longer than 15 characters.. dragons ahead")
	}

	if vlan != 0 {
		args = append(args, fmt.Sprintf("tag=%v", vlan))
	}

	if host {
		args = append(args, "--")
		args = append(args, "set")
		args = append(args, "Interface")
		args = append(args, tap)
		args = append(args, "type=internal")
	}

	if _, err := ovsCmdWrapper(args); err == errAlreadyExists {
		return err
	} else if err != nil {
		return fmt.Errorf("add port failed: %v", err)
	}

	return nil
}

// ovsDelPort removes a port from an openvswitch bridge.
func ovsDelPort(bridge, tap string) error {
	args := []string{
		"del-port",
		bridge,
		tap,
	}

	if _, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("remove port failed: %v", err)
	}

	return nil
}

// ovsCmdWrapper wraps `ovs-vsctl` commands, returning stdout, stderr, and any
// error produced running the command.
func ovsCmdWrapper(args []string) (string, error) {
	cmd := exec.Command("ovs-vsctl", args...)
	log.Debug("running ovs cmd: %v", cmd)

	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}

	if strings.Contains(string(out), "already exists") {
		return "", errAlreadyExists
	} else if strings.Contains(string(out), "no port named") {
		return "", errNoSuchPort
	}

	return "", fmt.Errorf("ovs cmd failed: %v %v", args, string(out))
}
