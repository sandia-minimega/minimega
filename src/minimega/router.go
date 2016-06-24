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
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
)

var (
	routers    map[int]*Router
	routerLock sync.Mutex
)

type Router struct {
	vmID      int        // local (and effectively unique regardless of namespace) vm id
	IPs       [][]string // positional ip address (index 0 is the first listed network in vm config net)
	lock      sync.Mutex
	logLevel  string
	updateIPs bool // only update IPs if we've made changes
	dhcp      map[string]*dhcp
}

type dhcp struct {
	addr   string
	low    string
	high   string
	router string
	dns    string
	static map[string]string
}

func (r *Router) String() string {
	r.lock.Lock()
	defer r.lock.Unlock()

	// create output
	var o bytes.Buffer
	fmt.Fprintf(&o, "IPs:\n")
	for i, v := range r.IPs {
		fmt.Fprintf(&o, "Network: %v: %v\n", i, v)
	}
	fmt.Fprintln(&o)

	if len(r.dhcp) > 0 {

		var keys []string
		for k, _ := range r.dhcp {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			d := r.dhcp[k]
			fmt.Fprintf(&o, "%v", d)
		}
		fmt.Fprintln(&o)
	}

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

	// only writeout ip changes if it's changed from the last commit in
	// order to avoid upsetting existing connections the device may have
	if r.updateIPs {
		// ips
		fmt.Fprintf(&out, "ip flush\n") // no need to manage state - just start over
		for i, v := range r.IPs {
			for _, w := range v {
				fmt.Fprintf(&out, "ip add %v %v\n", i, w)
			}
		}
	}

	// dnsmasq
	fmt.Fprintf(&out, "dnsmasq flush\n")
	for _, d := range r.dhcp {
		if d.low != "" {
			fmt.Fprintf(&out, "dnsmasq range %v %v %v\n", d.addr, d.low, d.high)
		}
		if d.router != "" {
			fmt.Fprintf(&out, "dnsmasq option router %v %v\n", d.addr, d.router)
		}
		if d.dns != "" {
			fmt.Fprintf(&out, "dnsmasq option dns %v %v\n", d.addr, d.dns)
		}
		for mac, ip := range d.static {
			fmt.Fprintf(&out, "dnsmasq static %v %v %v\n", d.addr, mac, ip)
		}
	}
	fmt.Fprintf(&out, "dnsmasq commit\n")

	filename := filepath.Join(*f_iomBase, fmt.Sprintf("minirouter-%v", r.vmID))
	return ioutil.WriteFile(filename, out.Bytes(), 0644)
}

// Create a new router for vm, or returns an existing router if it already
// exists
func FindOrCreateRouter(vm VM) *Router {
	log.Debug("FindOrCreateRouter: %v", vm)

	id := vm.GetID()
	if r, ok := routers[id]; ok {
		return r
	}
	r := &Router{
		vmID:     id,
		IPs:      [][]string{},
		logLevel: "error",
		dhcp:     make(map[string]*dhcp),
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

func (r *Router) Commit() error {
	log.Debugln("Commit")

	r.lock.Lock()
	defer r.lock.Unlock()

	// build a command list from the router
	err := r.generateConfig()
	if err != nil {
		return err
	}
	r.updateIPs = false // IPs are no longer stale

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

func (r *Router) LogLevel(level string) {
	log.Debug("RouterLogLevel: %v", level)

	r.lock.Lock()
	r.logLevel = level
	r.lock.Unlock()
}

func (r *Router) InterfaceAdd(n int, i string) error {
	log.Debug("RouterInterfaceAdd: %v, %v", n, i)

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
	r.updateIPs = true

	return nil
}

func (r *Router) InterfaceDel(n string, i string) error {
	log.Debug("RouterInterfaceDel: %v, %v", n, i)

	var network int
	var err error

	if n == "" {
		network = -1 // delete all interfaces on all networks
	} else {
		network, err = strconv.Atoi(n)
		if err != nil {
			return err
		}
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if network == -1 {
		r.IPs = make([][]string, len(r.IPs))
		r.updateIPs = true
		return nil
	}

	if network >= len(r.IPs) {
		return fmt.Errorf("no such network index: %v", network)
	}

	if i == "" {
		r.IPs[network] = []string{}
		r.updateIPs = true
		return nil
	}

	if !routerIsValidIP(i) {
		return fmt.Errorf("invalid IP: %v", i)
	}

	var found bool
	for j, v := range r.IPs[network] {
		if i == v {
			log.Debug("removing ip %v", i)
			r.IPs[network] = append(r.IPs[network][:j], r.IPs[network][j+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no such network: %v", i)
	}
	r.updateIPs = true

	return nil
}

func routerIsValidIP(i string) bool {
	if _, _, err := net.ParseCIDR(i); err != nil && i != "dhcp" {
		return false
	}
	return true
}

func (r *Router) DHCPAddRange(addr, low, high string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	d := r.dhcpFindOrCreate(addr)

	d.low = low
	d.high = high

	return nil
}

func (r *Router) DHCPAddRouter(addr, rtr string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	d := r.dhcpFindOrCreate(addr)

	d.router = rtr

	return nil
}

func (r *Router) DHCPAddDNS(addr, dns string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	d := r.dhcpFindOrCreate(addr)

	d.dns = dns

	return nil
}

func (r *Router) DHCPAddStatic(addr, mac, ip string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	d := r.dhcpFindOrCreate(addr)

	d.static[mac] = ip

	return nil
}

func (r *Router) dhcpFindOrCreate(addr string) *dhcp {
	if d, ok := r.dhcp[addr]; ok {
		return d
	}
	d := &dhcp{
		addr:   addr,
		static: make(map[string]string),
	}
	r.dhcp[addr] = d
	return d
}

func (d *dhcp) String() string {
	var o bytes.Buffer

	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)

	fmt.Fprintf(w, "Listen address:\t%v\n", d.addr)
	fmt.Fprintf(w, "Low address:\t%v\n", d.low)
	fmt.Fprintf(w, "High address:\t%v\n", d.high)
	fmt.Fprintf(w, "Router:\t%v\n", d.router)
	fmt.Fprintf(w, "DNS:\t%v\n", d.dns)
	fmt.Fprintf(w, "Static IPs:\t\n")
	w.Flush()

	w = new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)

	for mac, ip := range d.static {
		fmt.Fprintf(w, "\t%v\t%v\n", mac, ip)
	}
	w.Flush()

	return o.String()
}
