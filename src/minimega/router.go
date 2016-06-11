// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	log "minilog"
	"net"
	"sync"
	"text/tabwriter"
)

var (
	routers    map[int]*Router
	routerLock sync.Mutex
)

type Router struct {
	vmID int     // local (and unique regardless of namespace) vm id
	IPs  [][]*IP // positional ipv4 address (index 0 is the first listed network in vm config net)
	lock sync.Mutex
}

// a configured interface which can be in 2 states - an ipv4 or v6 address, or
// dhcp, otherwise the interface is taken down.
type IP struct {
	net  *net.IPNet
	dhcp bool
}

func (r *Router) String() string {
	r.lock.Lock()
	defer r.lock.Unlock()

	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "IPs:\t%v\n", r.IPs)
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

func (ip *IP) String() string {
	if ip.dhcp {
		return "dhcp"
	}
	return ip.net.String()
}

func init() {
	routers = make(map[int]*Router)
}

// routerCreate creates a new router for vm, or returns an existing router if
// it already exists
func routerCreate(vm VM) *Router {
	log.Debug("routerCreate: %v", vm)

	id := vm.GetID()
	if r, ok := routers[id]; ok {
		return r
	}
	r := &Router{
		vmID: id,
		IPs:  [][]*IP{},
	}
	nets := vm.GetNetworks()
	for i := 0; i < len(nets); i++ {
		r.IPs = append(r.IPs, []*IP{})
	}

	routers[id] = r

	return r
}

// FindRouter returns an existing router if it exists, otherwise nil
func FindRouter(vm VM) *Router {
	routerLock.Lock()
	defer routerLock.Unlock()

	id := vm.GetID()
	if r, ok := routers[id]; ok {
		return r
	}
	return nil
}

func RouterCommit(vm VM) error {
	log.Debug("routerCommit: %v", vm)

	routerLock.Lock()
	defer routerLock.Unlock()

	//r := routerCreate(vm)

	// build a command list from the router
	// c := r.generateConfig()

	// update cc commands for this router

	return nil
}

func RouterInterfaceAdd(vm VM, n int, i string) error {
	log.Debug("RouterInterfaceAdd: %v, %v, %v", vm, n, i)

	routerLock.Lock()
	r := routerCreate(vm)
	routerLock.Unlock()

	r.lock.Lock()
	defer r.lock.Unlock()

	if n >= len(r.IPs) {
		return fmt.Errorf("no such network index: %v", n)
	}

	ip, err := routerParseIP(i)
	if err != nil {
		return err
	}

	log.Debug("adding ip %v", ip)

	r.IPs[n] = append(r.IPs[n], ip)

	return nil
}

func RouterInterfaceDel(vm VM, n int, i string) error {
	log.Debug("RouterInterfaceDel: %v, %v, %v", vm, n, i)

	routerLock.Lock()
	r := routerCreate(vm)
	routerLock.Unlock()

	r.lock.Lock()
	defer r.lock.Unlock()

	if n >= len(r.IPs) {
		return fmt.Errorf("no such network index: %v", n)
	}

	ip, err := routerParseIP(i)
	if err != nil {
		return err
	}

	var found bool
	for j, v := range r.IPs[n] {
		if ip.String() == v.String() {
			log.Debug("removing ip %v", ip)
			r.IPs = append(r.IPs[:j], r.IPs[j+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no such network: %v", ip)
	}

	return nil
}

func routerParseIP(i string) (*IP, error) {
	ip := &IP{}

	if i == "dhcp" {
		ip.dhcp = true
	} else {
		var err error
		_, ip.net, err = net.ParseCIDR(i)
		if err != nil {
			return nil, err
		}
	}

	return ip, nil
}
