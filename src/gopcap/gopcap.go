// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// ipmac attempts to learn about active ip addresses associated with mac
// addresses on a particular interface, usually a bridge that can see data from
// many other interfaces. ipmac is used by creating a new ipmac object on a
// particular interface, and providing one or more MAC addresses to filter on.
package gopcap

// #cgo LDFLAGS: -lpcap
// #include <stdlib.h>
// #include "gopcap.h"
import "C"

import (
	"fmt"
	//	log "minilog"
	//	"strings"
	"sync"
	"unsafe"
)

type Pcap struct {
	handle       unsafe.Pointer
	dumperHandle unsafe.Pointer
	closed       bool
	lock         sync.Mutex
}

// NewLearner returns an IPMacLearner object bound to a particular interface.
func NewPCAP(dev string, file string) (*Pcap, error) {
	ret := &Pcap{}
	p := C.CString(dev)
	handle := C.pcapInit(p)
	C.free(unsafe.Pointer(p))
	if handle == nil {
		return ret, fmt.Errorf("could not open device %v", dev)
	}
	ret.handle = unsafe.Pointer(handle)

	// start pcap
	dumperHandle := C.pcapPrepare(handle, C.CString(file))
	C.free(unsafe.Pointer(p))
	if dumperHandle == nil {
		return ret, fmt.Errorf("could not open output file %v", file)
	}
	ret.dumperHandle = unsafe.Pointer(dumperHandle)

	go C.pcapCapture(handle, dumperHandle)

	return ret, nil
}

// Stop searching for IP addresses.
func (p *Pcap) Close() {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.closed = true
	C.pcapClose(p.handle, p.dumperHandle)
}
