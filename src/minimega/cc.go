// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	log "minilog"
	"os"
	"path/filepath"
	"ron"
	"strconv"
	"strings"
	"time"
)

const (
	CC_PORT          = 9002
	CC_SERIAL_PERIOD = 5
)

var (
	ccNode *ron.Server
	ccPort int
)

func ccMapPrefix(id int) {
	if ccPrefix != "" {
		ccPrefixMap[id] = ccPrefix
		log.Debug("prefix map %v: %v", id, ccPrefix)
	}
}

func ccUnmapPrefix(id int) {
	if prefix, ok := ccPrefixMap[id]; ok {
		delete(ccPrefixMap, id)
		log.Debug("prefix unmap %v: %v", id, prefix)
	}
}

func ccPrefixIDs(prefix string) []int {
	var ret []int
	for k, v := range ccPrefixMap {
		if v == prefix {
			ret = append(ret, k)
		}
	}
	return ret
}

func ccStart(portStr string) (err error) {
	if ccNode != nil {
		return fmt.Errorf("cc service already running")
	}

	ccPort = CC_PORT
	if portStr != "" {
		ccPort, err = strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port: %v", portStr)
		}
	}

	ccNode, err = ron.NewServer(ccPort, *f_iomBase)
	if err != nil {
		return fmt.Errorf("creating cc node %v", err)
	}

	log.Debug("created ron node at %v %v", ccPort, *f_base)
	return nil
}

func ccClear(what string) (err error) {
	log.Debug("cc clear %v", what)

	switch what {
	case "filter":
		ccFilter = nil
	case "commands":
		errs := []string{}
		for _, v := range ccNode.GetCommands() {
			err := ccNode.DeleteCommand(v.ID)
			if err != nil {
				errMsg := fmt.Sprintf("cc delete command %v : %v", v.ID, err)
				errs = append(errs, errMsg)
			}
			ccUnmapPrefix(v.ID)
		}
		if len(errs) != 0 {
			err = errors.New(strings.Join(errs, "\n"))
		}
	case "responses": // delete everything in miniccc_responses
		path := filepath.Join(*f_iomBase, ron.RESPONSE_PATH)
		err := os.RemoveAll(path)
		if err != nil {
			return err
		}
	case "prefix":
		ccPrefix = ""
	}

	return
}

func ccHasClient(c string) bool {
	return ccNode != nil && ccNode.HasClient(c)
}

func ccClients() map[string]bool {
	clients := make(map[string]bool)
	if ccNode != nil {
		c := ccNode.GetActiveClients()
		for k, _ := range c {
			clients[k] = true
		}
		return clients
	}
	return nil
}

// periodically check for VMs that we aren't dialed into with the ron serial
// service, and dial them.
func ccSerialWatcher() {
	log.Debugln("ccSerialWatcher")

	for {
		// get a list of every vm's serial port path
		hostPorts := vmGetFirstVirtioPort()

		// get a list of already opened serial port paths from ron
		ronPorts := ccNode.GetActiveSerialPorts()

		// find the difference
		var unconnected []string
		for _, v := range hostPorts {
			found := false
			for _, w := range ronPorts {
				if v == w {
					found = true
					break
				}
			}
			if !found {
				unconnected = append(unconnected, v)
			}
		}

		// dial the unconnected
		log.Debug("ccSerialWatcher connecting to: %v", unconnected)
		for _, v := range unconnected {
			err := ccNode.DialSerial(v)
			if err != nil {
				log.Errorln(err)
			}
		}

		time.Sleep(time.Duration(CC_SERIAL_PERIOD * time.Second))
	}
}

func filterString(f *ron.Client) string {
	if f == nil {
		return ""
	}

	var ret string

	var j []string
	if f.UUID != "" {
		j = append(j, "uuid="+f.UUID)
	}
	if f.Hostname != "" {
		j = append(j, "hostname="+f.Hostname)
	}
	if f.Arch != "" {
		j = append(j, "arch="+f.Arch)
	}
	if f.OS != "" {
		j = append(j, "os="+f.OS)
	}
	if len(f.IP) != 0 {
		for _, y := range f.IP {
			j = append(j, "ip="+y)
		}
	}
	if len(f.MAC) != 0 {
		for _, y := range f.MAC {
			j = append(j, "mac="+y)
		}
	}
	ret += strings.Join(j, " && ")

	return ret
}
