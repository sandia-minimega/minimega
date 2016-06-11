// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	log "minilog"
	"net"
	"path/filepath"
	"ron"
	"strings"
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
	ip   net.IP
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
	network := ip.net.String()
	nm := strings.Split(network, "/")
	return fmt.Sprintf("%v/%v", ip.ip.String(), nm[1])
}

func init() {
	routers = make(map[int]*Router)
}

func (r *Router) generateConfig() error {
	var out bytes.Buffer

	// ips
	for i, v := range r.IPs {
		for _, w := range v {
			fmt.Fprintf(&out, "ip %v %v\n", i, w)
		}
	}

	filename := filepath.Join(*f_iomBase, fmt.Sprintf("minirouter-%v", r.vmID))
	return ioutil.WriteFile(filename, out.Bytes(), 0644)
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

	vm.SetTag("minirouter", fmt.Sprintf("%v", id))

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
	r := routerCreate(vm)
	routerLock.Unlock()

	r.lock.Lock()
	defer r.lock.Unlock()

	// build a command list from the router
	err := r.generateConfig()
	if err != nil {
		return err
	}

	// remove any previous commands
	prefix := fmt.Sprintf("minirouter-%v", r.vmID)
	ids := ccPrefixIDs(prefix)
	if len(ids) != 0 {
		for _, v := range ids {
			c := ccNode.GetCommand(v)
			if c == nil {
				return fmt.Errorf("cc delete unknown command %v", v)
			}

			if !ccMatchNamespace(c) {
				// skip without warning
				continue
			}

			err := ccNode.DeleteCommand(v)
			if err != nil {
				return fmt.Errorf("cc delete command %v : %v", v, err)
			}
			ccUnmapPrefix(v)
		}
	}

	// filter on the minirouter tag
	filter := &ron.Client{
		Tags: make(map[string]string),
	}
	filter.Tags["minirouter"] = fmt.Sprintf("%v", r.vmID)

	// issue cc commands for this router
	cmd := &ron.Command{
		Filter: filter,
	}
	cmd.FilesSend = append(cmd.FilesSend, &ron.File{
		Name: prefix,
		Perm: 0644,
	})
	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)
	ccPrefixMap[id] = prefix

	cmd = &ron.Command{
		Filter:  filter,
		Command: []string{"minirouter", "-u", filepath.Join("/tmp/miniccc/files", prefix)},
	}
	id = ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)
	ccPrefixMap[id] = prefix

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
			r.IPs[n] = append(r.IPs[n][:j], r.IPs[n][j+1:]...)
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
		ip.ip, ip.net, err = net.ParseCIDR(i)
		if err != nil {
			return nil, err
		}
	}

	return ip, nil
}
