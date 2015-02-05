// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	log "minilog"
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
	ccNode *ron.Ron
)

func ccStart(portStr string) (err error) {
	port := CC_PORT
	if portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port: %v", portStr)
		}
	}

	ccNode, err = ron.New(port, ron.MODE_MASTER, "", *f_iomBase)
	if err != nil {
		return fmt.Errorf("creating cc node %v", err)
	}

	log.Debug("created ron node at %v %v", port, *f_base)
	return nil
}

func ccClear(what, idStr string) (err error) {
	log.Debug("cc clear -- %v:%v", what, idStr)
	var id int

	deleteAll := (idStr == Wildcard)
	if !deleteAll {
		id, err = strconv.Atoi(idStr)
		if err != nil {
			return fmt.Errorf("invalid id %v", idStr)
		}
	}

	if deleteAll {
		switch what {
		case "filter":
			ccFilters = make(map[int]*ron.Client)
		case "filesend":
			ccFileSend = make(map[int]string)
		case "filerecv":
			ccFileRecv = make(map[int]string)
		case "background":
			ccBackground = false
		case "record":
			ccRecord = false
		case "command":
			ccCommand = nil
		case "running":
			errs := []string{}
			for _, v := range ccNode.GetCommands() {
				err := ccNode.DeleteCommand(v.ID)
				if err != nil {
					errMsg := fmt.Sprintf("cc delete command %v : %v", v.ID, err)
					errs = append(errs, errMsg)
				}
			}
			err = errors.New(strings.Join(errs, "\n"))
		}
	} else {
		switch what {
		case "filter":
			if _, ok := ccFilters[id]; !ok {
				return fmt.Errorf("invalid filter id: %v", id)
			}
			delete(ccFilters, id)
		case "filesend":
			if _, ok := ccFileSend[id]; !ok {
				return fmt.Errorf("invalid file send id: %v", id)
			}
			delete(ccFileSend, id)
		case "filerecv":
			if _, ok := ccFileRecv[id]; !ok {
				return fmt.Errorf("invalid file recv id: %v", id)
			}
			delete(ccFileRecv, id)
		case "background":
			ccBackground = false
		case "record":
			ccRecord = false
		case "command":
			ccCommand = nil
		case "running":
			err = ccNode.DeleteCommand(id)
		}
	}

	return
}

func ccClients() map[string]bool {
	clients := make(map[string]bool)
	if ccNode != nil {
		c := ccNode.GetActiveClients()
		for _, v := range c {
			clients[v] = true
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
		hostPorts := vmGetAllSerialPorts()

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
			err := ccNode.SerialDialClient(v)
			if err != nil {
				log.Errorln(err)
			}
		}

		time.Sleep(time.Duration(CC_SERIAL_PERIOD * time.Second))
	}
}

func filterString(filter []*ron.Client) string {
	var ret string
	for _, f := range filter {
		if len(ret) != 0 {
			ret += " || "
		}
		ret += "( "
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
		ret += " )"
	}
	return ret
}
