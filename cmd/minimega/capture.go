// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/gonetflow"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type capture interface {
	Type() string
	Stop() error
}

type netflowConfig struct {
	gonetflow.Mode // embed

	Compress bool
}

type pcapCapture struct {
	bridge.CaptureConfig // embed

	// Bridge that is doing the capture
	Bridge string

	// ID returned by bridge
	ID int

	// Path where PCAP is being written
	Path string
}

type pcapVMCapture struct {
	pcapCapture // embed

	VM        VM
	Interface int
}

type pcapBridgeCapture struct {
	pcapCapture // embed
}

type netflowCapture struct {
	netflowConfig // embed

	// Bridge that is doing the capture
	Bridge string

	// Path where netflow is being written (may be host:port)
	Path string
}

type captures struct {
	m map[int]capture

	counter *Counter // counter simply for keys in m, not used otherwise

	bridge.CaptureConfig // embed config for new pcap captures
	netflowConfig        // embed config for new netflow captures
}

// Active timeout for connections in seconds. Due to a limitation with Open
// vSwitch, this has to be shared across namespaces.
var captureNFTimeout = 10

func (c *pcapCapture) Type() string {
	return "pcap"
}

func (c *pcapCapture) Stop() error {
	br, err := getBridge(c.Bridge)
	if err != nil {
		return err
	}

	return br.StopCapture(c.ID)
}

func (c *netflowCapture) Type() string {
	return "netflow"
}

func (c *netflowCapture) Stop() error {
	br, err := getBridge(c.Bridge)
	if err != nil {
		return err
	}

	// get the netflow object associated with this bridge
	nf, err := br.GetNetflow()
	if err != nil {
		return err
	}

	if err := nf.RemoveWriter(c.Path); err != nil {
		return err
	}

	// we were the last writer -- clean up the netflow object
	if !nf.HasWriter() {
		return br.DestroyNetflow()
	}

	return nil
}

// CaptureVM starts a new capture for a specified interface on a VM, writing
// the packets to the specified file in PCAP format.
func (c *captures) CaptureVM(vm VM, iface int, fname string) error {
	nic, err := vm.GetNetwork(iface)
	if err != nil {
		return err
	}

	bridge := nic.Bridge
	tap := nic.Tap

	br, err := getBridge(bridge)
	if err != nil {
		return err
	}

	id, err := br.CaptureTap(tap, fname, c.CaptureConfig)
	if err != nil {
		return err
	}

	return c.addCapture(&pcapVMCapture{
		pcapCapture: pcapCapture{
			CaptureConfig: c.CaptureConfig,
			Bridge:        bridge,
			Path:          fname,
			ID:            id,
		},
		VM:        vm,
		Interface: iface,
	})
}

// CaptureBridge starts a new capture for all the traffic on the specified
// bridge, writing all packets to the specified filename in PCAP format.
func (c *captures) CaptureBridge(bridge, fname string) error {
	br, err := getBridge(bridge)
	if err != nil {
		return err
	}

	id, err := br.Capture(fname, c.CaptureConfig)
	if err != nil {
		return err
	}

	return c.addCapture(&pcapBridgeCapture{
		pcapCapture: pcapCapture{
			CaptureConfig: c.CaptureConfig,
			Bridge:        bridge,
			Path:          fname,
			ID:            id,
		},
	})
}

// CaptureNetflowFile starts a new netflow recorder for all the traffic on the
// specified bridge, writing the netflow records to the specified filename.
func (c *captures) CaptureNetflowFile(bridge, fname string) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	if err := nf.NewFileWriter(fname, c.Mode, c.Compress); err != nil {
		return err
	}

	return c.addCapture(&netflowCapture{
		netflowConfig: c.netflowConfig,
		Bridge:        bridge,
		Path:          fname,
	})
}

// CaptureNetflowSocket starts a new netflow recorder for all the traffic on
// the specified bridge, writing the netflow record across the network to a
// remote host.
func (c *captures) CaptureNetflowSocket(bridge, transport, host string) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	if err := nf.NewSocketWriter(transport, host, c.Mode); err != nil {
		return err
	}

	return c.addCapture(&netflowCapture{
		netflowConfig: c.netflowConfig,
		Bridge:        bridge,
		Path:          fmt.Sprintf("%v:%v", transport, host),
	})
}

// addCapture generates an ID for the capture and adds it to the map.
func (c *captures) addCapture(v capture) error {
	c.m[c.counter.Next()] = v

	return nil
}

// StopAll stops all captures.
func (c *captures) StopAll() error {
	return c.stop(func(_ capture) bool {
		return true
	})
}

// StopVM stops capture for VM (wildcard supported).
func (c *captures) StopVM(s string) error {
	var found bool

	err := c.stop(func(v capture) bool {
		switch v := v.(type) {
		case *pcapVMCapture:
			r := v.VM.GetName() == s || s == Wildcard
			found = r || found
			return r
		}

		return false
	})

	if err == nil && !found {
		return vmNotFound(s)
	}
	return err
}

// StopBridge stops capture for bridge (wildcard supported).
func (c *captures) StopBridge(s, typ string) error {
	return c.stop(func(v capture) bool {
		if v.Type() != typ {
			return false
		}

		switch v := v.(type) {
		case *pcapBridgeCapture:
			return v.Bridge == s || s == Wildcard
		case *netflowCapture:
			return v.Bridge == s || s == Wildcard
		}

		return false
	})
}

// stop stops all captures that fn returns true for.
func (c *captures) stop(fn func(capture) bool) error {
	for id, v := range c.m {
		if fn(v) {
			if err := v.Stop(); err != nil {
				return err
			}

			delete(c.m, id)
		}
	}

	return nil
}

// getOrCreateNetflow wraps calls to getBridge and getNetflowFromBridge,
// creating each, if needed.
//
// Note that we cannot have a separate netflow object per namespace because OVS
// doesn't support more than one netflow object per bridge.
func getOrCreateNetflow(b string) (*gonetflow.Netflow, error) {
	br, err := getBridge(b)
	if err != nil {
		return nil, err
	}

	nf, err := br.GetNetflow()
	if err == bridge.ErrNoNetflow {
		return br.NewNetflow(captureNFTimeout)
	}

	return nf, nil
}

// updateNetflowTimeouts updates the timeouts for all netflow objects
func updateNetflowTimeouts() {
	for _, b := range bridges.Names() {
		br, err := getBridge(b)
		if err != nil {
			log.Error("could not get bridge: %v", err)
			continue
		}

		err = br.SetNetflowTimeout(captureNFTimeout)
		if err != nil && err != bridge.ErrNoNetflow {
			log.Error("unable to update netflow timeout: %v", err)
		}
	}
}
