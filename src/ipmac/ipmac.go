package ipmac

// ipmac attempts to learn about active ip addresses associated with mac
// addresses on a particular interface, usually a bridge that can see data from
// many other interfaces. ipmac is used by creating a new ipmac object on a
// particular interface, and providing one or more MAC addresses to filter on.

// #cgo LDFLAGS: -lpcap
// #include <stdlib.h>
// #include "ipmac.h"
import "C"

import (
	"fmt"
	"unsafe"
	log "minilog"
)

type IPMacLearner struct {
	handle unsafe.Pointer
	pairs  map[string]*IP
	closed bool
}

type IP struct {
	IP4 string
	IP6 string
}

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

	go ret.learner()

	return ret, nil
}

func (iml *IPMacLearner) GetMac(mac string) *IP {
	return iml.pairs[mac]
}

func (iml *IPMacLearner) AddMac(mac string) {
	log.Debugln("adding mac to filter:", mac)
	iml.pairs[mac] = &IP{}
	iml.updateFilter()
}

func (iml *IPMacLearner) DelMac(mac string) {
	delete(iml.pairs, mac)
	iml.updateFilter()
}

func (iml *IPMacLearner) Flush() {
	iml.pairs = make(map[string]*IP)
	iml.updateFilter()
}

func (iml *IPMacLearner) Close() {
	iml.closed = true
	C.pcapClose(iml.handle)
}

func (iml *IPMacLearner) updateFilter() {
	log.Debugln("updateFilter")
	filter := "(arp or (icmp6 and ip6[40] == 135)) and ("
	start := true
	for mac, _ := range iml.pairs {
		if !start {
			filter += "or "
		} else {
			start = false
		}
		filter += fmt.Sprintf("ether src %v ", mac)
	}
	filter += ")"
	log.Debugln("filter:", filter)

	p := C.CString(filter)
	C.pcapFilter(iml.handle, p)
	C.free(unsafe.Pointer(p))
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

		if _, ok := iml.pairs[mac]; !ok {
			continue
		}

		if ip != "" {
			iml.pairs[mac].IP4 = ip
		} else if ip6 != "" {
			iml.pairs[mac].IP6 = ip6
		}
	}
}
