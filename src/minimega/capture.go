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
	"strconv"
	"strings"
)

type capture struct {
	ID        int
	Type      string
	Bridge    string
	VM        int
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

func clearAllCaptures() (err error) {
	err = clearCapture("netflow", Wildcard)
	if err == nil {
		err = clearCapture("pcap", Wildcard)
	}

	return
}

func clearCapture(captureType, id string) (err error) {
	defer func() {
		// check if we need to remove the nf object
		if err != nil && captureType == "netflow" {
			err = cleanupNetflow()
		}
	}()

	if id == Wildcard {
		for _, v := range captureEntries {
			if v.Type == "pcap" && captureType == "pcap" {
				return stopPcapCapture(v)
			} else if v.Type == "netflow" && captureType == "netflow" {
				return stopNetflowCapture(v)
			}
		}
	} else {
		val, err := strconv.Atoi(id)
		if err != nil {
			return err
		}

		entry, ok := captureEntries[val]
		if !ok {
			return fmt.Errorf("entry %v does not exist", val)
		}

		if entry.Type != captureType {
			return fmt.Errorf("invalid id/capture type, `%s` != `%s`", entry.Type, captureType)
		} else if entry.Type == "pcap" {
			return stopPcapCapture(captureEntries[val])
		} else if entry.Type == "netflow" {
			return stopNetflowCapture(captureEntries[val])
		}
	}

	return nil
}

// startCapturePcap starts a new capture for a specified interface on a VM,
// writing the packets to the specified filename in PCAP format.
func startCapturePcap(vm string, iface int, filename string) error {
	// TODO: filter by namespace?
	// get the vm
	v := vms.FindVM(vm)
	if v == nil {
		return vmNotFound(vm)
	}

	config := getConfig(v)

	if len(config.Networks) <= iface {
		return fmt.Errorf("no such interface %v", iface)
	}

	tap := config.Networks[iface].Tap

	// attempt to start pcap on the bridge
	p, err := gopcap.NewPCAP(tap, filename)
	if err != nil {
		return err
	}

	// success! add it to the list
	ce := &capture{
		ID:        captureID.Next(),
		Type:      "pcap",
		VM:        v.GetID(),
		Interface: iface,
		Path:      filename,
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

	// attempt to start pcap on the mirrored tap
	p, err := gopcap.NewPCAP(tap, filename)
	if err != nil {
		return err
	}

	// success! add it to the list
	ce := &capture{
		ID:        captureID.Next(),
		Type:      "pcap",
		Bridge:    br.Name,
		VM:        -1,
		Interface: -1,
		Path:      filename,
		pcap:      p,
		tap:       tap,
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
