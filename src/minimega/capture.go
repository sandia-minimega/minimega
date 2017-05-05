// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bridge"
	"errors"
	"fmt"
	"gonetflow"
	"gopcap"
	log "minilog"
	"path/filepath"
)

type captures struct {
	m map[int]*capture

	counter *Counter
}

type capture struct {
	ID        int
	Type      string
	Bridge    string
	VM        VM
	Interface int
	Path      string
	Mode      string
	Compress  bool
	tap       string
	pcap      *gopcap.Pcap
}

var (
	captureNFTimeout = 10
)

// CapturePcap starts a new capture for a specified interface on a VM, writing
// the packets to the specified filename in PCAP format.
func (c *captures) CapturePcap(vm VM, iface int, filename string) error {
	networks := vm.GetNetworks()

	if len(networks) <= iface {
		return fmt.Errorf("no such interface %v", iface)
	}

	bridge := networks[iface].Bridge
	tap := networks[iface].Tap

	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(filename) {
		// TODO: should we capture to the VM directory instead?
		filename = filepath.Join(*f_iomBase, filename)
	}

	// attempt to start pcap on the bridge
	p, err := gopcap.NewPCAP(tap, filename)
	if err != nil {
		return err
	}

	// success!
	c.addCapture(&capture{
		Type:      "pcap",
		Bridge:    bridge,
		VM:        vm,
		Interface: iface,
		Path:      filename,
		Mode:      "N/A",
		pcap:      p,
	})

	return nil
}

// CapturePcapBridge starts a new capture for all the traffic on the specified
// bridge, writing all packets to the specified filename in PCAP format.
func (c *captures) CapturePcapBridge(b, filename string) error {
	// create the bridge if necessary
	br, err := getBridge(b)
	if err != nil {
		return err
	}

	tap, err := br.CreateMirror()
	if err != nil {
		return err
	}

	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(*f_iomBase, filename)
	}

	// attempt to start pcap on the mirrored tap
	p, err := gopcap.NewPCAP(tap, filename)
	if err != nil {
		return err
	}

	c.addCapture(&capture{
		Type:   "pcap",
		Bridge: br.Name,
		Path:   filename,
		Mode:   "N/A",
		pcap:   p,
		tap:    tap,
	})

	return nil
}

// CaptureNetflowFile starts a new netflow recorder for all the traffic on the
// specified bridge, writing the netflow records to the specified filename.
// Flags control whether the format is raw or ascii, and whether the logs will
// be compressed.
func (c *captures) CaptureNetflowFile(bridge, filename string, ascii, compress bool) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	mode := gonetflow.RAW
	if ascii {
		mode = gonetflow.ASCII
	}

	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(*f_iomBase, filename)
	}

	err = nf.NewFileWriter(filename, mode, compress)
	if err != nil {
		return err
	}

	c.addCapture(&capture{
		Type:     "netflow",
		Bridge:   bridge,
		Path:     filename,
		Mode:     mode.String(),
		Compress: compress,
	})

	return nil
}

// CaptureNetflowSocket starts a new netflow recorder for all the traffic on
// the specified bridge, writing the netflow record across the network to a
// remote host. The ascii flag controls whether the record format is raw or
// ascii.
func (c *captures) CaptureNetflowSocket(bridge, transport, host string, ascii bool) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	mode := gonetflow.RAW
	if ascii {
		mode = gonetflow.ASCII
	}

	err = nf.NewSocketWriter(transport, host, mode)
	if err != nil {
		return err
	}

	c.addCapture(&capture{
		Type:   "netflow",
		Bridge: bridge,
		Path:   fmt.Sprintf("%v:%v", transport, host),
		Mode:   mode.String(),
	})

	return nil
}

// addCapture generates an ID for the capture and adds it to the map.
func (c *captures) addCapture(v *capture) {
	v.ID = c.counter.Next()
	c.m[v.ID] = v
}

// StopAll stops all captures.
func (c *captures) StopAll() error {
	return c.stop(func(_ *capture) bool {
		return true
	})
}

// StopVM stops capture for VM (wildcard supported).
func (c *captures) StopVM(s, typ string) error {
	var found bool

	err := c.stop(func(v *capture) bool {
		if v.Type != typ || v.VM == nil {
			return false
		}

		r := v.VM.GetName() == s || s == Wildcard
		found = r || found
		return r
	})

	if err == nil && !found {
		return vmNotFound(s)
	}
	return err
}

// StopBridge stops capture for bridge (wildcard supported).
func (c *captures) StopBridge(s, typ string) error {
	return c.stop(func(v *capture) bool {
		if v.Type != typ || v.VM != nil {
			return false
		}

		return v.Bridge == s || s == Wildcard
	})
}

// stop stops all captures that fn returns true for.
func (c *captures) stop(fn func(*capture) bool) error {
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

func (c *capture) Stop() error {
	if c.Type == "pcap" {
		return c.stopPcap()
	} else if c.Type == "netflow" {
		return c.stopNetflow()
	}

	return errors.New("unknown capture type")
}

// stopPcap stops the specified pcap capture.
func (c *capture) stopPcap() error {
	if c.pcap == nil {
		return fmt.Errorf("capture %v has no valid pcap interface", c.ID)
	}

	// Do this from a separate goroutine to avoid a deadlock (issue #765). The
	// capture should end when we destroy the mirror.
	//
	// TODO: fix this properly.
	go c.pcap.Close()

	if c.tap != "" && c.Bridge != "" {
		br, err := getBridge(c.Bridge)
		if err != nil {
			return err
		}

		return br.DestroyMirror()
	}

	return nil
}

// stopNetflow stops the specified netflow capture.
func (c *capture) stopNetflow() error {
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

// getOrCreateNetflow returns the netflow object for the specified bridge,
// creating a new one if needed.
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
