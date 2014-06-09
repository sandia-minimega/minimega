// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"strconv"
	"sync"
)

type capture struct {
	ID       int
	Type     string
	Bridge   string
	Path     string
	Mode     string
	Compress bool
}

var (
	captureEntries map[int]*capture
	captureIDCount chan int
	captureLock    sync.Mutex
)

func init() {
	captureIDCount = make(chan int)
	for {
		count := 0
		captureIDCount <- count
		count++
	}
}

func cliCapture(c cliCommand) cliResponse {
	// capture must be:
	// capture
	// capture netflow <bridge> file <filename> <raw,ascii> [gzip]
	// capture netflow <bridge> socket <tcp,udp> <hostname:port> <raw,ascii>
	// capture clear netflow bridge <id,-1>
	log.Debugln("cliCapture")

	switch len(c.Args) {
	case 0:
		// print all info on all capture services
	case 5, 6:
		// new netflow capture
		if c.Args[0] != "netflow" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		if c.Args[2] != "file" && c.Args[2] != "socket" {
			return cliResponse{
				Error: "malformed command",
			}
		}
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

		// create the bridge if necessary
		b, err := getBridge(c.Args[1])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}

		nf, err := getNetflowFromBridge(c.Args[1])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		if nf == nil {
			// create a new netflow object
			nf, err = b.NewNetflow()
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
			mode, err := strconv.Atoi(c.Args[4])
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
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
			mode, err := strconv.Atoi(c.Args[5])
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
			err = nf.NewSocketWriter(c.Args[3], c.Args[4], mode)
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
	case 3:
		if c.Args[0] != "clear" || c.Args[1] != "netflow" {
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
		// TODO: remove nf object if no more netflow writers exist
	default:
		return cliResponse{
			Error: "malformed command",
		}
	}

	return cliResponse{}
}
