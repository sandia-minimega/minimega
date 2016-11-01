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
	"sync"
	"unsafe"
)

type Pcap struct {
	handle       unsafe.Pointer
	dumperHandle unsafe.Pointer
	done         chan bool
	lock         sync.Mutex
}

// NewLearner returns an IPMacLearner object bound to a particular interface.
func NewPCAP(dev string, file string) (*Pcap, error) {
	ret := &Pcap{
		done: make(chan bool),
	}

	devC := C.CString(dev)
	fileC := C.CString(file)
	defer C.free(unsafe.Pointer(devC))
	defer C.free(unsafe.Pointer(fileC))

	handle := C.gopcapInit(devC)
	if handle == nil {
		return ret, fmt.Errorf("could not open device %v", dev)
	}
	ret.handle = unsafe.Pointer(handle)

	// start pcap
	dumperHandle := C.gopcapPrepare(handle, fileC)
	if dumperHandle == nil {
		C.gopcapClose(ret.handle, nil)
		return nil, fmt.Errorf("could not open output file %v", file)
	}
	ret.dumperHandle = unsafe.Pointer(dumperHandle)

	go func() {
		C.gopcapCapture(handle, dumperHandle)
		close(ret.done)
	}()

	return ret, nil
}

// Stop searching for IP addresses.
func (p *Pcap) Close() {
	p.lock.Lock()
	defer p.lock.Unlock()

	C.gopcapClose(p.handle, p.dumperHandle)
	<-p.done

	p.handle = nil
	p.dumperHandle = nil
}
