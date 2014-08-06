// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"gonetflow"
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
	// capture pcap <bridge> <bridge name> <filename>
	// capture pcap <vm> <vm id> <tap> <filename>
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

func capturePcap(c cliCommand) cliResponse {
	// capture pcap <bridge> <bridge name> <filename>
	// capture pcap <vm> <vm id> <tap> <filename>
	// capture pcap [clear]
	// capture pcap clear <id, -1>
	if len(c.Args) == 1 {
		// capture pcap, generate output
		captureLock.Lock()
		defer captureLock.Unlock()
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "ID\tBridge\tVM/interface\tPath\tCompress\n")
		for _, v := range captureEntries {
			if v.Type == "pcap" {
				fmt.Fprintf(w, "%v\t%v\t%v/%v\t%v\t%v\n", v.ID, v.Bridge, v.VM, v.Interface, v.Path, v.Compress)
			}
		}
		w.Flush()

		out := o.String()

		return cliResponse{
			Response: out,
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
		captureLock.Lock()
		defer captureLock.Unlock()
		if c.Args[2] == "-1" {
			for k, v := range captureEntries {
				if v.Type == "netflow" {
					// get the netflow object associated with this bridge
					nf, err := getNetflowFromBridge(v.Bridge)
					if err != nil {
						return cliResponse{
							Error: err.Error(),
						}
					}
					err = nf.RemoveWriter(v.Path)
					if err != nil {
						return cliResponse{
							Error: err.Error(),
						}
					}
					delete(captureEntries, k)
				}
			}
		} else {
			val, err := strconv.Atoi(c.Args[2])
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
			if v, ok := captureEntries[val]; !ok {
				return cliResponse{
					Error: fmt.Sprintf("entry %v does not exist", val),
				}
			} else {
				// get the netflow object associated with this bridge
				nf, err := getNetflowFromBridge(v.Bridge)
				if err != nil {
					return cliResponse{
						Error: err.Error(),
					}
				}
				err = nf.RemoveWriter(v.Path)
				if err != nil {
					return cliResponse{
						Error: err.Error(),
					}
				}
				delete(captureEntries, val)
			}
		}

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
				return cliResponse{
					Error: err.Error(),
				}
			}

			err = b.DestroyNetflow()
			if err != nil {
				if !strings.Contains(err.Error(), "has no netflow object") {
					return cliResponse{
						Error: err.Error(),
					}
				}
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
