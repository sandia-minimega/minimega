// Copyright (2016) Sandia Corporation.
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
	"text/tabwriter"
)

var (
	routers map[int]*Router = make(map[int]*Router)
)

type Router struct {
	vmID         int        // local (and effectively unique regardless of namespace) vm id
	IPs          [][]string // positional ip address (index 0 is the first listed network in vm config net)
	logLevel     string
	updateIPs    bool // only update IPs if we've made changes
	dhcp         map[string]*dhcp
	dns          map[string]string
	rad          map[string]bool // using a bool placeholder here for later RAD options
	staticRoutes map[string]string
	ospfRoutes   map[string]*ospf
}

type ospf struct {
	area       string
	interfaces map[string]bool
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
			fmt.Fprintf(&o, "%v\n", d)
		}
	}

	if len(r.dns) > 0 {
		fmt.Fprintf(&o, "DNS:\n")
		var keys []string
		for k, _ := range r.dns {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, ip := range keys {
			host := r.dns[ip]
			fmt.Fprintf(&o, "%v\t%v\n", ip, host)
		}
		fmt.Fprintln(&o)
	}

	if len(r.rad) > 0 {
		fmt.Fprintf(&o, "Router Advertisements:\n")
		var keys []string
		for k, _ := range r.rad {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, subnet := range keys {
			fmt.Fprintf(&o, "%v\n", subnet)
		}
		fmt.Fprintln(&o)
	}

	if len(r.staticRoutes) > 0 {
		fmt.Fprintf(&o, "Static Routes:\n")
		var keys []string
		for k, _ := range r.staticRoutes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, network := range keys {
			fmt.Fprintf(&o, "%v\t%v\n", network, r.staticRoutes[network])
		}
		fmt.Fprintln(&o)
	}

	if len(r.ospfRoutes) > 0 {
		var keys []string
		for k, _ := range r.ospfRoutes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			ospfRoute := r.ospfRoutes[k]
			fmt.Fprintf(&o, "%v\n", ospfRoute)
		}
	}

	vm := vms.FindVM(fmt.Sprintf("%v", r.vmID))
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
			fmt.Fprintf(&out, "dnsmasq dhcp range %v %v %v\n", d.addr, d.low, d.high)
		}
		if d.router != "" {
			fmt.Fprintf(&out, "dnsmasq dhcp option router %v %v\n", d.addr, d.router)
		}
		if d.dns != "" {
			fmt.Fprintf(&out, "dnsmasq dhcp option dns %v %v\n", d.addr, d.dns)
		}
		for mac, ip := range d.static {
			fmt.Fprintf(&out, "dnsmasq dhcp static %v %v %v\n", d.addr, mac, ip)
		}
	}
	for ip, host := range r.dns {
		fmt.Fprintf(&out, "dnsmasq dns %v %v\n", ip, host)
	}
	for subnet, _ := range r.rad {
		fmt.Fprintf(&out, "dnsmasq ra %v\n", subnet)
	}
	fmt.Fprintf(&out, "dnsmasq commit\n")

	// bird
	fmt.Fprintf(&out, "bird flush\n")
	for network, nh := range r.staticRoutes {
		fmt.Fprintf(&out, "bird static %v %v\n", network, nh)
	}
	for _, o := range r.ospfRoutes {
		for iface, _ := range o.interfaces {
			fmt.Fprintf(&out, "bird ospf %v %v\n", o.area, iface)
		}
	}
	fmt.Fprintf(&out, "bird commit\n")

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
		vmID:         id,
		IPs:          [][]string{},
		logLevel:     "error",
		dhcp:         make(map[string]*dhcp),
		dns:          make(map[string]string),
		rad:          make(map[string]bool),
		staticRoutes: make(map[string]string),
		ospfRoutes:   make(map[string]*ospf),
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
	id := vm.GetID()
	return routers[id]
}

func (r *Router) Commit() error {
	log.Debugln("Commit")

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
		Filter:  filter,
		Command: []string{"rm", filepath.Join("/tmp/miniccc/files", prefix)},
	}
	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)
	ccPrefixMap[id] = prefix

	cmd = &ron.Command{
		Filter: filter,
	}
	cmd.FilesSend = append(cmd.FilesSend, &ron.File{
		Name: prefix,
		Perm: 0644,
	})
	id = ccNode.NewCommand(cmd)
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

	r.logLevel = level
}

func (r *Router) InterfaceAdd(n int, i string) error {
	log.Debug("RouterInterfaceAdd: %v, %v", n, i)

	if n >= len(r.IPs) {
		return fmt.Errorf("no such network index: %v", n)
	}

	if !routerIsValidIP(i) {
		return fmt.Errorf("invalid IP: %v", i)
	}

	for _, v := range r.IPs[n] {
		if v == i {
			return fmt.Errorf("IP %v already exists", i)
		}
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
	d := r.dhcpFindOrCreate(addr)

	d.low = low
	d.high = high

	return nil
}

func (r *Router) DHCPAddRouter(addr, rtr string) error {
	d := r.dhcpFindOrCreate(addr)

	d.router = rtr

	return nil
}

func (r *Router) DHCPAddDNS(addr, dns string) error {
	d := r.dhcpFindOrCreate(addr)

	d.dns = dns

	return nil
}

func (r *Router) DHCPAddStatic(addr, mac, ip string) error {
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

	var keys []string
	for k, _ := range d.static {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, mac := range keys {
		ip := d.static[mac]
		fmt.Fprintf(w, "\t%v\t%v\n", mac, ip)
	}
	w.Flush()

	return o.String()
}

func (r *Router) DNSAdd(ip, hostname string) {
	r.dns[ip] = hostname
}

func (r *Router) DNSDel(ip string) error {
	if ip == "" {
		r.dns = make(map[string]string)
	} else if _, ok := r.dns[ip]; ok {
		delete(r.dns, ip)
	} else {
		return fmt.Errorf("no such ip: %v", ip)
	}
	return nil
}

func (r *Router) RADAdd(subnet string) {
	r.rad[subnet] = true
}

func (r *Router) RADDel(subnet string) error {
	if subnet == "" {
		r.rad = make(map[string]bool)
	} else {
		if _, ok := r.rad[subnet]; ok {
			delete(r.rad, subnet)
		} else {
			return fmt.Errorf("no such subnet: %v", subnet)
		}
	}
	return nil
}

func (r *Router) RouteStaticAdd(network, nh string) {
	r.staticRoutes[network] = nh
}

func (r *Router) RouteStaticDel(network string) error {
	if network == "" {
		r.staticRoutes = make(map[string]string)
	} else {
		if _, ok := r.staticRoutes[network]; ok {
			delete(r.staticRoutes, network)
		} else {
			return fmt.Errorf("no such network: %v", network)
		}
	}
	return nil
}

func (r *Router) ospfFindOrCreate(area string) *ospf {
	if o, ok := r.ospfRoutes[area]; ok {
		return o
	}
	o := &ospf{
		area:       area,
		interfaces: make(map[string]bool),
	}
	r.ospfRoutes[area] = o
	return o
}

func (o *ospf) String() string {
	var out bytes.Buffer

	fmt.Fprintf(&out, "OSPF Area:\t%v\n", o.area)
	fmt.Fprintf(&out, "Interfaces:\n")

	var keys []string
	for k, _ := range o.interfaces {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, iface := range keys {
		fmt.Fprintf(&out, "\t%v\n", iface)
	}

	return out.String()
}

func (r *Router) RouteOSPFAdd(area, iface string) {
	o := r.ospfFindOrCreate(area)
	o.interfaces[iface] = true
}

func (r *Router) RouteOSPFDel(area, iface string) error {
	if area == "" {
		r.ospfRoutes = make(map[string]*ospf)
		return nil
	}

	o, ok := r.ospfRoutes[area]
	if !ok {
		return fmt.Errorf("no such area: %v", area)
	}

	if iface == "" {
		o.interfaces = make(map[string]bool)
		return nil
	}

	if _, ok := o.interfaces[iface]; ok {
		delete(o.interfaces, iface)
		return nil
	}

	return fmt.Errorf("no such interface: %v", iface)
}
