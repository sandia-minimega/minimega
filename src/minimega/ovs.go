// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	log "minilog"
	"os/exec"
	"strings"
)

var (
	ErrAlreadyExists = errors.New("already exists")
	ErrNoSuchPort    = errors.New("no such port")
)

// ovsAddBridge creates a new OVS bridge. Returns whether the bridge was new or
// not, or any error that occurred.
func ovsAddBridge(name string) (bool, error) {
	args := []string{
		"add-br",
		name,
	}

	_, sErr, err := ovsCmdWrapper(args)
	if err == ErrAlreadyExists {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("ovsAddBridge: %v: %v", err, sErr)
	}

	return true, nil
}

// ovsDelBridge deletes a OVS bridge.
func ovsDelBridge(name string) error {
	args := []string{
		"del-br",
		name,
	}

	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("ovsDelBridge: %v: %v", err, sErr)
	}

	return nil
}

func ovsAddPort(bridge, tap string, lan int, host bool) error {
	args := []string{
		"add-port",
		bridge,
		tap,
	}

	if lan != TrunkVLAN {
		args = append(args, fmt.Sprintf("tag=%v", lan))
	}

	if host {
		args = append(args, "--")
		args = append(args, "set")
		args = append(args, "Interface")
		args = append(args, tap)
		args = append(args, "type=internal")
	}

	if _, sErr, err := ovsCmdWrapper(args); err == ErrAlreadyExists {
		return ErrAlreadyExists
	} else if err != nil {
		return fmt.Errorf("ovsAddPort: %v: %v", err, sErr)
	}

	return nil
}

func ovsDelPort(bridge, tap string) error {
	args := []string{
		"del-port",
		bridge,
		tap,
	}

	_, sErr, err := ovsCmdWrapper(args)
	if err != nil {
		return fmt.Errorf("ovsDelPort: %v: %v", err, sErr)
	}

	return nil
}

func ovsGetTaps() []string {
	var tapList []string

	args := []string{
		"show",
	}

	sOut, _, _ := ovsCmdWrapper(args)

	taps := strings.Split(sOut, "\n")

	for _, t := range taps {
		if strings.Contains(t, "Port") &&
			strings.Contains(t, "mega_tap") {

			name := strings.Split(t, "\"")
			if len(name) > 1 {
				tapList = append(tapList, name[1])
			}
		}
	}

	return tapList
}

func ovsCmdWrapper(args []string) (string, string, error) {
	ovsLock.Lock()
	defer ovsLock.Unlock()

	var sOut bytes.Buffer
	var sErr bytes.Buffer

	cmd := exec.Command(process("ovs"), args...)
	cmd.Stdout = &sOut
	cmd.Stderr = &sErr
	log.Debug("running ovs cmd: %v", cmd)

	if err := cmdTimeout(cmd, OVS_TIMEOUT); err != nil {
		if strings.Contains(sErr.String(), "already exists") {
			err = ErrAlreadyExists
		} else if strings.Contains(sErr.String(), "no port named") {
			err = ErrNoSuchPort
		} else {
			log.Error("openvswitch cmd failed: %v %v", cmd, sErr.String())
		}

		return sOut.String(), sErr.String(), err
	}

	return sOut.String(), sErr.String(), nil
}
