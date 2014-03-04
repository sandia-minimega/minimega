// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// ipmac attempts to learn about active ip addresses associated with mac
// addresses on a particular interface, usually a bridge that can see data from
// many other interfaces. ipmac is used by creating a new ipmac object on a
// particular interface, and providing one or more MAC addresses to filter on.
package ipmac

// #cgo LDFLAGS: -lpcap
// #include <stdlib.h>
// #include "ipmac.h"
import "C"

import (
	"fmt"
	log "minilog"
	"strings"
	"sync"
	"unsafe"
)

type IPMacLearner struct {
	handle unsafe.Pointer
	pairs  map[string]*IP
	closed bool
	lock   sync.Mutex
}

type IP struct {
	IP4 string // string representation of the IPv4 address, if known
	IP6 string // string representation of the IPv6 address, if known
}

// NewLearner returns an IPMacLearner object bound to a particular interface.
func NewLearner(dev string) (*IPMacLearner, error) {
	ret := &IPMacLearner{
		pairs: make(map[string]*IP),
	}
	p := C.CString(dev)
	handle := C.pcapInit(p)
	C.free(unsafe.Pointer(p))
	if handle == nil {
		return ret, fmt.Errorf("could not open device %v", dev)
	}
	ret.handle = unsafe.Pointer(handle)

	filter := "(arp or (icmp6 and ip6[40] == 135))"
	p = C.CString(filter)
	C.pcapFilter(ret.handle, p)
	C.free(unsafe.Pointer(p))

	go ret.learner()

	return ret, nil
}

// Lookup any known IPv4 or IPv6 addresses associated with a given MAC address.
func (iml *IPMacLearner) GetIPFromMac(mac string) *IP {
	return iml.pairs[mac]
}

// Add a MAC address to the list of addresses to search for. IPMacLearner will
// not gather information on MAC addresses not in the list.
func (iml *IPMacLearner) AddMac(mac string) {
	iml.lock.Lock()
	defer iml.lock.Unlock()
	log.Debugln("adding mac to filter:", mac)
	iml.pairs[mac] = &IP{}
}

// Delete a MAC address from the list of addresses to search for.
func (iml *IPMacLearner) DelMac(mac string) {
	iml.lock.Lock()
	defer iml.lock.Unlock()
	delete(iml.pairs, mac)
}

// Remove all MAC addresses from the search list.
func (iml *IPMacLearner) Flush() {
	iml.lock.Lock()
	defer iml.lock.Unlock()
	iml.pairs = make(map[string]*IP)
}

// Stop searching for IP addresses.
func (iml *IPMacLearner) Close() {
	iml.lock.Lock()
	defer iml.lock.Unlock()
	iml.closed = true
	C.pcapClose(iml.handle)
}

func (iml *IPMacLearner) learner() {
	for {
		pair := C.pcapRead(iml.handle)
		if pair == nil {
			if iml.closed {
				return
			}
			continue
		}
		mac := C.GoString(pair.mac)

		var ip, ip6 string
		if pair.ip != nil {
			ip = C.GoString(pair.ip)
		}
		if pair.ip6 != nil {
			ip6 = C.GoString(pair.ip6)
		}

		log.Debug("got mac/ip pair:", mac, ip, ip6)

		iml.lock.Lock()

		// skip macs we aren't tracking
		if _, ok := iml.pairs[mac]; !ok {
			iml.lock.Unlock()
			continue
		}

		if ip != "" {
			iml.pairs[mac].IP4 = ip
		} else if ip6 != "" {
			if iml.pairs[mac].IP6 != "" && strings.HasPrefix(ip6, "fe80") {
				log.Debugln("ignoring link-local over existing IPv6 address")
			} else {
				iml.pairs[mac].IP6 = ip6
			}
		}

		iml.lock.Unlock()
	}
}
