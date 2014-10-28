// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"gonetflow"
	"gopcap"
	log "minilog"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
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
	captureIDCount = make(chan int)
	count := 0
	go func() {
		for {
			captureIDCount <- count
			count++
		}
	}()
}

func cliCapture(c cliCommand) cliResponse {
	// capture must be:
	// capture
	// capture pcap bridge <bridge name> <filename>
	// capture pcap vm <vm id> <tap> <filename>
	// capture pcap [clear]
	// capture pcap clear <id, -1>
	// capture netflow <bridge> file <filename> <raw,ascii> [gzip]
	// capture netflow <bridge> socket <tcp,udp> <hostname:port> <raw,ascii>
	// capture netflow clear <id,-1>
	// capture netflow timeout [time]
	// capture netflow
	log.Debugln("cliCapture")

	if len(c.Args) == 0 {
		// create output for all capture types
		captureLock.Lock()
		defer captureLock.Unlock()
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "ID\tType\tBridge\tVM/interface\tPath\tMode\tCompress\n")
		for _, v := range captureEntries {
			fmt.Fprintf(w, "%v\t%v\t%v\t%v/%v\t%v\t%v\t%v\n", v.ID, v.Type, v.Bridge, v.VM, v.Interface, v.Path, v.Mode, v.Compress)
		}
		w.Flush()

		// get netflow stats for each bridge
		var nfstats string
		b := enumerateBridges()
		for _, v := range b {
			nf, err := getNetflowFromBridge(v)
			if err != nil {
				if !strings.Contains(err.Error(), "has no netflow object") {
					return cliResponse{
						Error: err.Error(),
					}
				}
				continue
			}
			nfstats += fmt.Sprintf("Bridge %v:\n", v)
			nfstats += fmt.Sprintf("minimega listening on port: %v\n", nf.GetPort())
			nfstats += nf.GetStats()
		}

		out := o.String() + "\n" + nfstats

		return cliResponse{
			Response: out,
		}
		return cliResponse{}
	}

	switch c.Args[0] {
	case "pcap":
		return capturePcap(c)
	case "netflow":
		return captureNetflow(c)
	default:
		return cliResponse{
			Error: "malformed command",
		}
	}
}

func clearCapture(captureType, id string) error {
	captureLock.Lock()
	defer captureLock.Unlock()
	if id == "-1" {
		for k, v := range captureEntries {
			if v.Type == "pcap" && captureType == "pcap" {
				delete(captureEntries, k)
				if v.pcap != nil {
					v.pcap.Close()
				} else {
					return fmt.Errorf("capture %v has no valid pcap interface", k)
				}
				if v.tap != "" && v.Bridge != "" {
					b, err := getBridge(v.Bridge)
					if err != nil {
						return err
					}
					err = b.DeleteBridgeMirror(v.tap)
					if err != nil {
						return err
					}
				}
			} else if v.Type == "netflow" && captureType == "netflow" {
				delete(captureEntries, k)
				// get the netflow object associated with this bridge
				nf, err := getNetflowFromBridge(v.Bridge)
				if err != nil {
					return err
				}
				err = nf.RemoveWriter(v.Path)
				if err != nil {
					return err
				}
			}
		}
	} else {
		val, err := strconv.Atoi(id)
		if err != nil {
			return err
		}
		if v, ok := captureEntries[val]; !ok {
			return errors.New(fmt.Sprintf("entry %v does not exist", val))
		} else {
			if v.Type == "pcap" && captureType == "pcap" {
				delete(captureEntries, val)
				if v.pcap != nil {
					v.pcap.Close()
				} else {
					return fmt.Errorf("capture %v has no valid pcap interface", val)
				}
				if v.tap != "" && v.Bridge != "" {
					b, err := getBridge(v.Bridge)
					if err != nil {
						return err
					}
					err = b.DeleteBridgeMirror(v.tap)
					if err != nil {
						return err
					}
				}
			} else if v.Type == "netflow" && captureType == "netflow" {
				delete(captureEntries, val)
				// get the netflow object associated with this bridge
				nf, err := getNetflowFromBridge(v.Bridge)
				if err != nil {
					return err
				}
				err = nf.RemoveWriter(v.Path)
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("entry %v is not a pcap capture", val)
			}
		}
	}

	if captureType == "netflow" {
		// check if we need to remove the nf object
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
	}

	return nil
}

func capturePcap(c cliCommand) cliResponse {
	// capture pcap bridge <bridge name> <filename>
	// capture pcap vm <vm id> <tap> <filename>
	// capture pcap clear <id, -1>
	if len(c.Args) == 1 {
		// capture pcap, generate output
		captureLock.Lock()
		defer captureLock.Unlock()
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "ID\tBridge\tVM/interface\tPath\n")
		for _, v := range captureEntries {
			if v.Type == "pcap" {
				fmt.Fprintf(w, "%v\t%v\t%v/%v\t%v\n", v.ID, v.Bridge, v.VM, v.Interface, v.Path)
			}
		}
		w.Flush()

		out := o.String()

		return cliResponse{
			Response: out,
		}
	}

	switch c.Args[1] {
	case "clear":
		if len(c.Args) != 3 {
			return cliResponse{
				Error: "malformed command",
			}
		}

		err := clearCapture("pcap", c.Args[2])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
	case "vm":
		if len(c.Args) != 5 {
			return cliResponse{
				Error: "malformed command",
			}
		}

		// get the vm
		v := vms.getVM(c.Args[2])
		if v == nil {
			return cliResponse{
				Error: fmt.Sprintf("no such vm %v", c.Args[2]),
			}
		}

		// get the interface by index
		val, err := strconv.Atoi(c.Args[3])
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("vm id %v : %v", c.Args[3], err),
			}
		}

		if len(v.taps) < val {
			return cliResponse{
				Error: fmt.Sprintf("no such interface %v", val),
			}
		}

		tap := v.taps[val]

		// attempt to start pcap on the bridge
		p, err := gopcap.NewPCAP(tap, c.Args[4])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}

		// success! add it to the list
		ce := &capture{
			ID:        <-captureIDCount,
			Type:      "pcap",
			VM:        v.Id,
			Interface: val,
			Path:      c.Args[4],
			pcap:      p,
		}

		captureLock.Lock()
		captureEntries[ce.ID] = ce
		captureLock.Unlock()
	case "bridge":
		if len(c.Args) != 4 {
			return cliResponse{
				Error: "malformed command",
			}
		}

		// create the bridge if necessary
		b, err := getBridge(c.Args[2])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}

		tap, err := b.CreateBridgeMirror()
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}

		// attempt to start pcap on the mirrored tap
		p, err := gopcap.NewPCAP(tap, c.Args[3])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}

		// success! add it to the list
		ce := &capture{
			ID:        <-captureIDCount,
			Type:      "pcap",
			Bridge:    c.Args[2],
			VM:        -1,
			Interface: -1,
			Path:      c.Args[3],
			pcap:      p,
			tap:       tap,
		}

		captureLock.Lock()
		captureEntries[ce.ID] = ce
		captureLock.Unlock()
	default:
		return cliResponse{
			Error: "malformed command",
		}
	}

	return cliResponse{}
}

func captureNetflow(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 1:
		// create output
		captureLock.Lock()
		defer captureLock.Unlock()
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "ID\tBridge\tPath\tMode\tCompress\n")
		for _, v := range captureEntries {
			if v.Type == "netflow" {
				fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\n", v.ID, v.Bridge, v.Path, v.Mode, v.Compress)
			}
		}
		w.Flush()

		// get netflow stats for each bridge
		var nfstats string
		b := enumerateBridges()
		for _, v := range b {
			nf, err := getNetflowFromBridge(v)
			if err != nil {
				if !strings.Contains(err.Error(), "has no netflow object") {
					return cliResponse{
						Error: err.Error(),
					}
				}
				continue
			}
			nfstats += fmt.Sprintf("Bridge %v:\n", v)
			nfstats += fmt.Sprintf("minimega listening on port: %v\n", nf.GetPort())
			nfstats += nf.GetStats()
		}

		out := o.String() + "\n" + nfstats

		return cliResponse{
			Response: out,
		}
	case 2:
		if c.Args[0] != "netflow" || c.Args[1] != "timeout" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		return cliResponse{
			Response: fmt.Sprintf("%v", captureNFTimeout),
		}
	case 3:
		if c.Args[0] == "netflow" && c.Args[1] == "timeout" {
			val, err := strconv.Atoi(c.Args[2])
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}

			captureNFTimeout = val
			captureUpdateNFTimeouts()
			return cliResponse{}
		}

		if c.Args[0] != "netflow" || c.Args[1] != "clear" {
			return cliResponse{
				Error: "malformed command",
			}
		}

		// delete by id or -1 for all netflow writers
		err := clearCapture("netflow", c.Args[2])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
	case 5, 6:
		// new netflow capture
		if c.Args[0] != "netflow" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		if c.Args[2] == "file" {
			if c.Args[4] != "raw" && c.Args[4] != "ascii" {
				return cliResponse{
					Error: "malformed command",
				}
			}
			if len(c.Args) == 6 && c.Args[5] != "gzip" {
				return cliResponse{
					Error: "malformed command",
				}
			}
		} else if c.Args[2] == "socket" {
			if c.Args[3] != "tcp" && c.Args[3] != "udp" {
				return cliResponse{
					Error: "malformed command",
				}
			}
			if c.Args[5] != "raw" && c.Args[5] != "ascii" {
				return cliResponse{
					Error: "malformed command",
				}
			}
		} else {
			return cliResponse{
				Error: "malformed command",
			}
		}

		// create the bridge if necessary
		b, err := getBridge(c.Args[1])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}

		nf, err := getNetflowFromBridge(c.Args[1])
		if err != nil {
			if !strings.Contains(err.Error(), "has no netflow object") {
				return cliResponse{
					Error: err.Error(),
				}
			}
		}
		if nf == nil {
			// create a new netflow object
			nf, err = b.NewNetflow(captureNFTimeout)
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
		}

		// create the writer for this entry
		switch c.Args[2] {
		case "file":
			var compress bool
			if len(c.Args) == 6 {
				compress = true
			}
			var mode int
			if c.Args[4] == "ascii" {
				mode = gonetflow.ASCII
			} else {
				mode = gonetflow.RAW
			}
			err = nf.NewFileWriter(c.Args[3], mode, compress)
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}

			ce := &capture{
				ID:       <-captureIDCount,
				Type:     "netflow",
				Bridge:   c.Args[1],
				Path:     c.Args[3],
				Mode:     c.Args[4],
				Compress: compress,
			}

			captureLock.Lock()
			captureEntries[ce.ID] = ce
			captureLock.Unlock()
		case "socket":
			var mode int
			if c.Args[5] == "ascii" {
				mode = gonetflow.ASCII
			} else {
				mode = gonetflow.RAW
			}
			err := nf.NewSocketWriter(c.Args[3], c.Args[4], mode)
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}

			ce := &capture{
				ID:     <-captureIDCount,
				Type:   "netflow",
				Bridge: c.Args[1],
				Path:   fmt.Sprintf("%v:%v", c.Args[3], c.Args[4]),
				Mode:   c.Args[5],
			}

			captureLock.Lock()
			captureEntries[ce.ID] = ce
			captureLock.Unlock()
		}
	default:
		return cliResponse{
			Error: "malformed command",
		}
	}

	return cliResponse{}
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

func cliCaptureClear() error {
	err := clearCapture("netflow", "-1")
	if err != nil {
		return err
	}
	err = clearCapture("pcap", "-1")
	if err != nil {
		return err
	}
	return nil
}
