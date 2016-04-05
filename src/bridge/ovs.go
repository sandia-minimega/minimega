// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"bytes"
	"errors"
	"fmt"
	log "minilog"
	"os/exec"
	"strings"
	"time"
)

var (
	errAlreadyExists = errors.New("already exists")
	errNoSuchPort    = errors.New("no such port")
)

// timeout for openvswitch commands.
const ovsTimeout = time.Duration(5 * time.Second)

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

	_, sErr, err := ovsCmdWrapper(args)
	if err == errAlreadyExists {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("add bridge failed: %v: %v", err, sErr)
	}

	return true, nil
}

// ovsDelBridge deletes a openvswitch bridge.
func ovsDelBridge(name string) error {
	args := []string{
		"del-br",
		name,
	}

	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("delete bridge failed: %v: %v", err, sErr)
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

	if _, sErr, err := ovsCmdWrapper(args); err == errAlreadyExists {
		return err
	} else if err != nil {
		return fmt.Errorf("add port failed: %v: %v", err, sErr)
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

	_, sErr, err := ovsCmdWrapper(args)
	if err != nil {
		return fmt.Errorf("remove port failed: %v: %v", err, sErr)
	}

	return nil
}

// ovsCmdWrapper wraps `ovs-vsctl` commands, returning stdout, stderr, and any
// error produced running the command. Commands are run with a timeout of
// ovsTimeout.
func ovsCmdWrapper(args []string) (string, string, error) {
	var sOut bytes.Buffer
	var sErr bytes.Buffer

	cmd := exec.Command("ovs-vsctl", args...)
	cmd.Stdout = &sOut
	cmd.Stderr = &sErr
	log.Debug("running ovs cmd: %v", cmd)

	if err := cmdTimeout(cmd, ovsTimeout); err != nil {
		if strings.Contains(sErr.String(), "already exists") {
			err = errAlreadyExists
		} else if strings.Contains(sErr.String(), "no port named") {
			err = errNoSuchPort
		} else {
			log.Error("openvswitch cmd failed: %v %v", cmd, sErr.String())
		}

		return sOut.String(), sErr.String(), err
	}

	return sOut.String(), sErr.String(), nil
}
