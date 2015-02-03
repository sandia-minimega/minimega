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
	"sync"
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
	captureEntries   map[int]*capture
	captureIDCount   chan int
	captureLock      sync.Mutex
	captureNFTimeout int
)

func init() {
	captureNFTimeout = 10
	captureEntries = make(map[int]*capture)
	captureIDCount = makeIDChan()
}

func clearAllCaptures() (err error) {
	err = clearCapture("netflow", "*")
	if err == nil {
		err = clearCapture("pcap", "*")
	}

	return
}

func clearCapture(captureType, id string) (err error) {
	captureLock.Lock()
	defer captureLock.Unlock()

	defer func() {
		// check if we need to remove the nf object
		if err != nil && captureType == "netflow" {
			err = cleanupNetflow()
		}
	}()

	if id == "*" {
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
	// get the vm
	v := vms.findVm(vm)
	if v == nil {
		return vmNotFound(vm)
	}

	if len(v.taps) <= iface {
		return fmt.Errorf("no such interface %v", iface)
	}

	tap := v.taps[iface]

	// attempt to start pcap on the bridge
	p, err := gopcap.NewPCAP(tap, filename)
	if err != nil {
		return err
	}

	// success! add it to the list
	ce := &capture{
		ID:        <-captureIDCount,
		Type:      "pcap",
		VM:        v.Id,
		Interface: iface,
		Path:      filename,
		pcap:      p,
	}

	captureLock.Lock()
	captureEntries[ce.ID] = ce
	captureLock.Unlock()

	return nil
}

// startBridgeCapturePcap starts a new capture for all the traffic on the
// specified bridge, writing all packets to the specified filename in PCAP
// format.
func startBridgeCapturePcap(bridge, filename string) error {
	// create the bridge if necessary
	b, err := getBridge(bridge)
	if err != nil {
		return err
	}

	tap, err := b.CreateBridgeMirror()
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
		ID:        <-captureIDCount,
		Type:      "pcap",
		Bridge:    bridge,
		VM:        -1,
		Interface: -1,
		Path:      filename,
		pcap:      p,
		tap:       tap,
	}

	captureLock.Lock()
	captureEntries[ce.ID] = ce
	captureLock.Unlock()

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
		ID:       <-captureIDCount,
		Type:     "netflow",
		Bridge:   bridge,
		Path:     filename,
		Mode:     modeStr,
		Compress: compress,
	}

	captureLock.Lock()
	captureEntries[ce.ID] = ce
	captureLock.Unlock()

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
		ID:     <-captureIDCount,
		Type:   "netflow",
		Bridge: bridge,
		Path:   fmt.Sprintf("%v:%v", transport, host),
		Mode:   modeStr,
	}

	captureLock.Lock()
	captureEntries[ce.ID] = ce
	captureLock.Unlock()

	return nil
}

// stopPcapCapture stops the specified pcap capture. Assumes that captureLock
// has been acquired by the caller.
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
		b, err := getBridge(entry.Bridge)
		if err != nil {
			return err
		}

		return b.DeleteBridgeMirror(entry.tap)
	}

	return nil
}

// stopNetflowCapture stops the specified netflow capture. Assumes that
// captureLock has been acquired by the caller.
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
	b := enumerateBridges()
	for _, v := range b {
		empty := true
		for _, n := range captureEntries {
			if n.Bridge == v {
				empty = false
				break
			}
		}

		if !empty {
			continue
		}

		b, err := getBridge(v)
		if err != nil {
			return err
		}

		err = b.DestroyNetflow()
		if err != nil {
			if !strings.Contains(err.Error(), "has no netflow object") {
				return err
			}
		}
	}

	return nil
}

// getOrCreateNetflow wraps calls to getBridge and getNetflowFromBridge,
// creating each, if needed.
func getOrCreateNetflow(bridge string) (*gonetflow.Netflow, error) {
	// create the bridge if necessary
	b, err := getBridge(bridge)
	if err != nil {
		return nil, err
	}

	nf, err := getNetflowFromBridge(bridge)
	if err != nil && !strings.Contains(err.Error(), "has no netflow object") {
		return nil, err
	}

	if nf == nil {
		// create a new netflow object
		nf, err = b.NewNetflow(captureNFTimeout)
	}

	return nf, err
}

func captureUpdateNFTimeouts() {
	b := enumerateBridges()
	for _, v := range b {
		br, err := getBridge(v)
		if err != nil {
			log.Error("could not get bridge: %v", err)
			continue
		}
		_, err = getNetflowFromBridge(v)
		if err != nil {
			if !strings.Contains(err.Error(), "has no netflow object") {
				log.Error("get netflow object from bridge: %v", err)
			}
			continue
		}
		err = br.UpdateNFTimeout(captureNFTimeout)
		if err != nil {
			log.Error("update netflow timeout: %v", err)
		}
	}
}
