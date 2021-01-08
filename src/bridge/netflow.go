// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"
	"gonetflow"
	log "minilog"
)

var ErrNoNetflow = errors.New("bridge has no netflow object")

// NewNetflow creates a new netflow for the bridge.
func (b *Bridge) NewNetflow(timeout int) (*gonetflow.Netflow, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("creating netflow on bridge %v", b.Name)

	if b.nf != nil {
		return nil, fmt.Errorf("bridge already has a netflow object")
	}

	nf, port, err := gonetflow.NewNetflow()
	if err != nil {
		return nil, err
	}

	// connect openvswitch to our new netflow object
	args := []string{
		"--",
		"set",
		"Bridge",
		b.Name,
		"netflow=@nf",
		"--",
		"--id=@nf",
		"create",
		"NetFlow",
		fmt.Sprintf("targets=\"127.0.0.1:%v\"", port),
		fmt.Sprintf("active-timeout=%v", timeout),
	}

	if _, err := ovsCmdWrapper(args); err != nil {
		return nil, fmt.Errorf("enable netflow failed: %v", err)
	}

	b.nf = nf

	return nf, nil
}

// GetNetflow returns the active netflow for the bridge.
func (b *Bridge) GetNetflow() (*gonetflow.Netflow, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	if b.nf == nil {
		return nil, ErrNoNetflow
	}

	return b.nf, nil
}

// DestroyNetflow destroys the active netflow.
func (b *Bridge) DestroyNetflow() error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return b.destroyNetflow()
}

func (b *Bridge) destroyNetflow() error {
	log.Info("destroying netflow on %v", b.Name)

	if b.nf == nil {
		return ErrNoNetflow
	}

	b.nf.Stop()

	// disconnect openvswitch from netflow object
	args := []string{
		"clear",
		"Bridge",
		b.Name,
		"netflow",
	}

	if _, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("destroy netflow failed: %v", err)
	}

	b.nf = nil

	return nil
}

// SetNetflowTimeout updates the timeout on the active netflow.
func (b *Bridge) SetNetflowTimeout(timeout int) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	if b.nf == nil {
		return ErrNoNetflow
	}

	args := []string{
		"set",
		"NetFlow",
		b.Name,
		fmt.Sprintf("active_timeout=%v", timeout),
	}
	if _, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("set netflow timeout failed: %v", err)
	}

	return nil
}
