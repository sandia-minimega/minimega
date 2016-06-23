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
	vmID     int        // local (and effectively unique regardless of namespace) vm id
	IPs      [][]string // positional ip address (index 0 is the first listed network in vm config net)
	lock     sync.Mutex
	logLevel string
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

	vm := vms[r.vmID]
	if vm == nil { // this really shouldn't ever happen
		log.Error("could not find vm: %v", r.vmID)
		return ""
	}

	lines := strings.Split(vm.Tag("minirouter_log"), "\n")

	fmt.Fprintln(&o, "Log:")
	for _, v := range lines {
		fmt.Fprintf(&o, "\t%v\n", v)
	}

	return o.String()
}

func init() {
	routers = make(map[int]*Router)
}

func (r *Router) generateConfig() error {
	var out bytes.Buffer

	// log level
	fmt.Fprintf(&out, "log level %v\n", r.logLevel)

	// ips
	fmt.Fprintf(&out, "ip flush\n") // no need to manage state - just start over
	for i, v := range r.IPs {
		for _, w := range v {
			fmt.Fprintf(&out, "ip add %v %v\n", i, w)
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
		vmID:     id,
		IPs:      [][]string{},
		logLevel: "error",
	}
	nets := vm.GetNetworks()
	for i := 0; i < len(nets); i++ {
		r.IPs = append(r.IPs, []string{})
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

func RouterLogLevel(vm VM, level string) {
	log.Debug("RouterLogLevel: %v, %v", vm, level)

	routerLock.Lock()
	r := routerCreate(vm)
	routerLock.Unlock()

	r.lock.Lock()
	r.logLevel = level
	r.lock.Unlock()
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

	if !routerIsValidIP(i) {
		return fmt.Errorf("invalid IP: %v", i)
	}

	log.Debug("adding ip %v", i)

	r.IPs[n] = append(r.IPs[n], i)

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

	if !routerIsValidIP(i) {
		return fmt.Errorf("invalid IP: %v", i)
	}

	var found bool
	for j, v := range r.IPs[n] {
		if i == v {
			log.Debug("removing ip %v", i)
			r.IPs[n] = append(r.IPs[n][:j], r.IPs[n][j+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no such network: %v", i)
	}

	return nil
}

func routerIsValidIP(i string) bool {
	if _, _, err := net.ParseCIDR(i); err != nil && i != "dhcp" {
		return false
	}
	return true
}
