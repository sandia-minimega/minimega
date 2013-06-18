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
	log "minilog"
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
	iml.lock.Lock()
	defer iml.lock.Unlock()
	log.Debugln("adding mac to filter:", mac)
	iml.pairs[mac] = &IP{}
	iml.updateFilter()
}

func (iml *IPMacLearner) DelMac(mac string) {
	iml.lock.Lock()
	defer iml.lock.Unlock()
	delete(iml.pairs, mac)
	iml.updateFilter()
}

func (iml *IPMacLearner) Flush() {
	iml.lock.Lock()
	defer iml.lock.Unlock()
	iml.pairs = make(map[string]*IP)
	iml.updateFilter()
}

func (iml *IPMacLearner) Close() {
	iml.lock.Lock()
	defer iml.lock.Unlock()
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

		iml.lock.Lock()

		if _, ok := iml.pairs[mac]; !ok {
			iml.lock.Unlock()
			continue
		}

		if ip != "" {
			iml.pairs[mac].IP4 = ip
		} else if ip6 != "" {
			iml.pairs[mac].IP6 = ip6
		}

		iml.lock.Unlock()
	}
}
