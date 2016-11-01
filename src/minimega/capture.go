// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"gonetflow"
	"gopcap"
	log "minilog"
	"path/filepath"
	"strings"
)

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
	captureEntries   = make(map[int]*capture)
	captureID        = NewCounter()
	captureNFTimeout = 10
)

func (c *capture) InNamespace(namespace string) bool {
	if namespace == "" || c.VM == nil {
		return true
	}

	return c.VM.GetNamespace() == namespace
}

func (c *capture) Stop() error {
	if c.Type == "pcap" {
		return stopPcapCapture(c)
	} else if c.Type == "netflow" {
		return stopNetflowCapture(c)
	}

	return errors.New("unknown capture type")
}

func clearAllCaptures() error {
	// run all the clears, even if there are errors on some
	return makeErrSlice([]error{
		clearCapture("netflow", "bridge", Wildcard),
		clearCapture("pcap", "bridge", Wildcard),
		clearCapture("netflow", "vm", Wildcard),
		clearCapture("pcap", "vm", Wildcard),
	})
}

func clearCapture(captureType, bridgeOrVM, name string) (err error) {
	defer func() {
		// check if we need to remove the nf object
		if err != nil && captureType == "netflow" {
			err = cleanupNetflow()
		}
	}()

	namespace := GetNamespaceName()

	var foundOne bool

	for _, v := range captureEntries {
		// should match current namespace
		if !v.InNamespace(namespace) {
			continue
		}

		// should match the capture type we're clearing
		if v.Type != captureType {
			continue
		}

		// make sure we're clearing the right types
		if v.VM == nil && bridgeOrVM == "vm" {
			// v is a bridge capture but they specified vms
			continue
		} else if v.VM != nil && bridgeOrVM == "bridge" {
			// v is a vm capture but they specified bridges
			continue
		}

		if name != Wildcard {
			// make sure the name matches
			if v.VM != nil && v.VM.GetName() != name {
				continue
			} else if v.VM == nil && v.Bridge != name {
				continue
			}
		}

		foundOne = true

		if err := v.Stop(); err != nil {
			return err
		}
	}

	// we made it through the loop and didn't find what we were trying to clear
	if name == Wildcard || foundOne {
		return
	}

	switch bridgeOrVM {
	case "vm":
		return vmNotFound(name)
	case "bridge":
		return fmt.Errorf("no capture of type %v on bridge %v", captureType, name)
	}

	return nil
}

// startCapturePcap starts a new capture for a specified interface on a VM,
// writing the packets to the specified filename in PCAP format.
func startCapturePcap(v string, iface int, filename string) error {
	// get the vm
	vm := vms.FindVM(v)
	if vm == nil {
		return vmNotFound(v)
	}

	config := getConfig(vm)

	if len(config.Networks) <= iface {
		return fmt.Errorf("no such interface %v", iface)
	}

	bridge := config.Networks[iface].Bridge
	tap := config.Networks[iface].Tap

	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(*f_iomBase, filename)
	}

	// attempt to start pcap on the bridge
	p, err := gopcap.NewPCAP(tap, filename)
	if err != nil {
		return err
	}

	// success! add it to the list
	ce := &capture{
		ID:        captureID.Next(),
		Type:      "pcap",
		Bridge:    bridge,
		VM:        vm,
		Interface: iface,
		Path:      filename,
		Mode:      "N/A",
		pcap:      p,
	}

	captureEntries[ce.ID] = ce

	return nil
}

// startBridgeCapturePcap starts a new capture for all the traffic on the
// specified bridge, writing all packets to the specified filename in PCAP
// format.
func startBridgeCapturePcap(b, filename string) error {
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

	// success! add it to the list
	ce := &capture{
		ID:     captureID.Next(),
		Type:   "pcap",
		Bridge: br.Name,
		Path:   filename,
		Mode:   "N/A",
		pcap:   p,
		tap:    tap,
	}

	captureEntries[ce.ID] = ce

	return nil
}

// startCaptureNetflowFile starts a new netflow recorder for all the traffic on
// the specified bridge, writing the netflow records to the specified filename.
// Flags control whether the format is raw or ascii, and whether the logs will
// be compressed.
func startCaptureNetflowFile(bridge, filename string, ascii, compress bool) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	mode := gonetflow.RAW
	modeStr := "raw"
	if ascii {
		mode = gonetflow.ASCII
		modeStr = "ascii"
	}

	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(*f_iomBase, filename)
	}

	err = nf.NewFileWriter(filename, mode, compress)
	if err != nil {
		return err
	}

	ce := &capture{
		ID:       captureID.Next(),
		Type:     "netflow",
		Bridge:   bridge,
		Path:     filename,
		Mode:     modeStr,
		Compress: compress,
	}

	captureEntries[ce.ID] = ce

	return nil
}

// startCaptureNetflowSocket starts a new netflow recorder for all the traffic
// on the specified bridge, writing the netflow record across the network to a
// remote host. The ascii flag controls whether the record format is raw or
// ascii.
func startCaptureNetflowSocket(bridge, transport, host string, ascii bool) error {
	nf, err := getOrCreateNetflow(bridge)
	if err != nil {
		return err
	}

	mode := gonetflow.RAW
	modeStr := "raw"
	if ascii {
		mode = gonetflow.ASCII
		modeStr = "ascii"
	}

	err = nf.NewSocketWriter(transport, host, mode)
	if err != nil {
		return err
	}

	ce := &capture{
		ID:     captureID.Next(),
		Type:   "netflow",
		Bridge: bridge,
		Path:   fmt.Sprintf("%v:%v", transport, host),
		Mode:   modeStr,
	}

	captureEntries[ce.ID] = ce

	return nil
}

// stopPcapCapture stops the specified pcap capture.
func stopPcapCapture(entry *capture) error {
	if entry.Type != "pcap" {
		return errors.New("called stop pcap capture on capture of wrong type")
	}

	delete(captureEntries, entry.ID)

	if entry.pcap == nil {
		return fmt.Errorf("capture %v has no valid pcap interface", entry.ID)
	}
	entry.pcap.Close()

	if entry.tap != "" && entry.Bridge != "" {
		br, err := getBridge(entry.Bridge)
		if err != nil {
			return err
		}

		return br.DestroyMirror()
	}

	return nil
}

// stopNetflowCapture stops the specified netflow capture.
func stopNetflowCapture(entry *capture) error {
	if entry.Type != "netflow" {
		return errors.New("called stop netflow capture on capture of wrong type")
	}

	delete(captureEntries, entry.ID)

	// get the netflow object associated with this bridge
	nf, err := getNetflowFromBridge(entry.Bridge)
	if err != nil {
		return err
	}

	return nf.RemoveWriter(entry.Path)
}

// cleanupNetflow destroys any netflow objects that are not currently
// capturing. This should be invoked after calling stopNetflowCapture.
func cleanupNetflow() error {
outer:
	for _, b := range bridges.Names() {
		// Check that there aren't any captures still using the netflow
		for _, n := range captureEntries {
			if n.Bridge == b {
				continue outer
			}
		}

		br, err := getBridge(b)
		if err != nil {
			return err
		}

		err = br.DestroyNetflow()
		if err != nil && !strings.Contains(err.Error(), "has no netflow object") {
			return err
		}
	}

	return nil
}

func getNetflowFromBridge(b string) (*gonetflow.Netflow, error) {
	br, err := getBridge(b)
	if err != nil {
		return nil, err
	}

	return br.GetNetflow()
}

// getOrCreateNetflow wraps calls to getBridge and getNetflowFromBridge,
// creating each, if needed.
func getOrCreateNetflow(b string) (*gonetflow.Netflow, error) {
	// create the bridge if necessary
	br, err := getBridge(b)
	if err != nil {
		return nil, err
	}

	nf, err := br.GetNetflow()
	if err != nil && !strings.Contains(err.Error(), "has no netflow object") {
		return nil, err
	} else if nf == nil {
		// create a new netflow object
		nf, err = br.NewNetflow(captureNFTimeout)
	}

	return nf, err
}

func captureUpdateNFTimeouts() {
	for _, b := range bridges.Names() {
		br, err := getBridge(b)
		if err != nil {
			log.Error("could not get bridge: %v", err)
			continue
		}

		err = br.SetNetflowTimeout(captureNFTimeout)
		if err != nil && !strings.Contains(err.Error(), "has no netflow object") {
			log.Error("unable to update netflow timeout: %v", err)
		}
	}
}
