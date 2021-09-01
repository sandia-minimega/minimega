// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	log "minilog"
	"ron"
)

type Router struct {
	vm           VM
	IPs          [][]string     // positional ip address (index 0 is the first listed network in vm config net)
	Loopbacks    map[int]string //loopback where it takes index number from interface command as key
	logLevel     string
	updateIPs    bool // only update IPs if we've made changes
	dhcp         map[string]*dhcp
	dns          map[string][]string
	upstream     string
	gw           string
	rad          map[string]bool // using a bool placeholder here for later RAD options
	staticRoutes map[string]string
	namedRoutes  map[string]map[string]string
	ospfRoutes   map[string]*ospf
	bgpRoutes    map[string]*bgp
	routerID     string
	FW           *fw
}

type ospf struct {
	area       string
	interfaces map[string]map[string]string
	prefixes   map[string]bool
}

type dhcp struct {
	addr   string
	low    string
	high   string
	router string
	dns    string
	static map[string]string
}

type bgp struct {
	processName    string
	localIP        string
	localAS        int
	neighborIP     string
	neighborAs     int
	routeReflector bool
	exportNetworks map[string]bool
}

type fw struct {
	defaultAction string
	rules         [][]*fwRule // firewall rule per network interface index
	chains        map[string]*fwChain
}

type fwChain struct {
	defaultAction string
	rules         []*fwRule
}

type fwRule struct {
	in     bool // if true then in, if false then out - not used for chain rules
	src    string
	dst    string
	proto  string
	action string // will be accept/drop/reject, or the name of a chain
}

// NewRouter creates a new router with a given number of interfaces,
// initializing all the maps and setting sane defaults.
func NewRouter(i int) *Router {
	r := &Router{
		IPs:          make([][]string, i),
		Loopbacks:    make(map[int]string),
		logLevel:     "error",
		dhcp:         make(map[string]*dhcp),
		dns:          make(map[string][]string),
		rad:          make(map[string]bool),
		staticRoutes: make(map[string]string),
		namedRoutes:  make(map[string]map[string]string),
		ospfRoutes:   make(map[string]*ospf),
		bgpRoutes:    make(map[string]*bgp),
		routerID:     "0.0.0.0",

		FW: &fw{
			defaultAction: "accept",
			rules:         make([][]*fwRule, i),
			chains:        make(map[string]*fwChain),
		},
	}
	return r
}

// Create a new router for vm, or returns an existing router if it already
// exists
func (ns *Namespace) FindOrCreateRouter(vm VM) *Router {
	log.Debug("FindOrCreateRouter: %v", vm)

	id := vm.GetID()
	if r, ok := ns.routers[id]; ok {
		return r
	}

	r := NewRouter(len(vm.GetNetworks()))
	r.vm = vm

	ns.routers[id] = r

	vm.SetTag("minirouter", fmt.Sprintf("%v", id))

	return r
}

// FindRouter returns an existing router if it exists, otherwise nil
func (ns *Namespace) FindRouter(vm VM) *Router {
	id := vm.GetID()
	return ns.routers[id]
}

// ToString for Router Object
func (r *Router) String() string {
	// create output
	var o bytes.Buffer
	// IP config
	fmt.Fprintf(&o, "IPs:\n")
	for i, v := range r.IPs {
		fmt.Fprintf(&o, "Network: %v: %v\n", i, v)
	}

	if len(r.Loopbacks) > 0 {
		var keys []string
		for _, k := range r.Loopbacks {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(&o, "Loopback IPs:\n")
		for _, l := range keys {
			fmt.Fprintf(&o, "%v\n", l)
		}
	}

	fmt.Fprintln(&o)
	// DHCP config
	if len(r.dhcp) > 0 {
		var keys []string
		for k := range r.dhcp {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			d := r.dhcp[k]
			fmt.Fprintf(&o, "%v\n", d)
		}
	}
	// DNS Config
	if len(r.dns) > 0 {
		fmt.Fprintf(&o, "DNS:\n")
		var keys []string
		for k := range r.dns {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, ip := range keys {
			hosts := r.dns[ip]
			for _, host := range hosts {
				fmt.Fprintf(&o, "%v\t%v\n", ip, host)
			}
		}
		fmt.Fprintln(&o)
	}

	if r.upstream != "" {
		fmt.Fprintf(&o, "Upstream DNS: %v\n", r.upstream)
	}

	if r.gw != "" {
		fmt.Fprintf(&o, "Default Gateway: %v\n", r.gw)
	}

	if len(r.rad) > 0 {
		fmt.Fprintf(&o, "Router Advertisements:\n")
		var keys []string
		for k := range r.rad {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, subnet := range keys {
			fmt.Fprintf(&o, "%v\n", subnet)
		}
		fmt.Fprintln(&o)
	}
	// Static route config
	if len(r.staticRoutes) > 0 {
		fmt.Fprintf(&o, "Static Routes:\n")
		var keys []string
		for k := range r.staticRoutes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, network := range keys {
			fmt.Fprintf(&o, "%v\t%v\n", network, r.staticRoutes[network])
		}
		fmt.Fprintln(&o)
	}

	if len(r.namedRoutes) > 0 {
		fmt.Fprintf(&o, "Named Static Routes:\n")
		var names []string
		for k := range r.namedRoutes {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&o, "%v\n", name)
			for network, nh := range r.namedRoutes[name] {
				if nh != "" {
					nh = "\t" + nh
				}
				fmt.Fprintf(&o, "\t%v%v\n", network, nh)
			}
		}
		fmt.Fprintln(&o)
	}
	// OSPF route config
	if len(r.ospfRoutes) > 0 {
		var keys []string
		for k := range r.ospfRoutes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			ospfRoute := r.ospfRoutes[k]
			fmt.Fprintf(&o, "%v\n", ospfRoute)
		}
	}
	// BGP route config
	if len(r.bgpRoutes) > 0 {
		var keys []string
		for k := range r.bgpRoutes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			bgpRoute := r.bgpRoutes[k]
			fmt.Fprintf(&o, "%v\n", bgpRoute)
		}
	}
	if r.vm != nil {
		lines := strings.Split(r.vm.Tag("minirouter_log"), "\n")
		fmt.Fprintln(&o, "Log:")
		for _, v := range lines {
			fmt.Fprintf(&o, "\t%v\n", v)
		}
	}

	return o.String()
}

// Write config performs the write operation to the Router configfile
func (r *Router) writeConfig(w io.Writer) error {
	// log level
	fmt.Fprintf(w, "log level %v\n", r.logLevel)

	// only writeout ip changes if it's changed from the last commit in
	// order to avoid upsetting existing connections the device may have
	if r.updateIPs {
		// ips
		fmt.Fprintf(w, "ip flush\n") // no need to manage state - just start over
		for i, j := range r.IPs {
			for _, k := range j {
				fmt.Fprintf(w, "ip add %v %v\n", i, k)
			}
		}
		for _, v := range r.Loopbacks {
			fmt.Fprintf(w, "ip add lo %v\n", v)
		}
	}

	// dnsmasq
	fmt.Fprintf(w, "dnsmasq flush\n")
	for _, d := range r.dhcp {
		if d.low != "" {
			fmt.Fprintf(w, "dnsmasq dhcp range %v %v %v\n", d.addr, d.low, d.high)
		}
		if d.router != "" {
			fmt.Fprintf(w, "dnsmasq dhcp option router %v %v\n", d.addr, d.router)
		}
		if d.dns != "" {
			fmt.Fprintf(w, "dnsmasq dhcp option dns %v %v\n", d.addr, d.dns)
		}
		for mac, ip := range d.static {
			fmt.Fprintf(w, "dnsmasq dhcp static %v %v %v\n", d.addr, mac, ip)
		}
	}
	for ip, hosts := range r.dns {
		for _, host := range hosts {
			fmt.Fprintf(w, "dnsmasq dns %v %v\n", ip, host)
		}
	}
	if r.upstream != "" {
		fmt.Fprintf(w, "dnsmasq upstream %v\n", r.upstream)
	}
	if r.gw != "" {
		fmt.Fprintf(w, "route add default gw %v\n", r.gw)
	} else {
		fmt.Fprintf(w, "route del default\n")
	}
	for subnet := range r.rad {
		fmt.Fprintf(w, "dnsmasq ra %v\n", subnet)
	}
	fmt.Fprintf(w, "dnsmasq commit\n")

	// bird
	fmt.Fprintf(w, "bird flush\n")
	for network, nh := range r.staticRoutes {
		fmt.Fprintf(w, "bird static %v %v %v\n", network, nh, "null")
	}
	for name, network := range r.namedRoutes {
		for nt, nh := range network {
			if nh == "" {
				nh = "null"
			}
			fmt.Fprintf(w, "bird static %v %v %v\n", nt, nh, name)
		}
	}
	for _, o := range r.ospfRoutes {
		for iface, options := range o.interfaces {
			// ensure interface is created, even if there are no options
			fmt.Fprintf(w, "bird ospf %v %v\n", o.area, iface)
			// set all the options
			for k, v := range options {
				fmt.Fprintf(w, "bird ospf %v %v %q %q\n", o.area, iface, k, v)
			}
		}
		for filter := range o.prefixes {
			fmt.Fprintf(w, "bird ospf %v filter %v\n", o.area, filter)
		}
	}
	for _, b := range r.bgpRoutes {
		fmt.Fprintf(w, "bird bgp %v local %v %v\n", b.processName, b.localIP, b.localAS)
		fmt.Fprintf(w, "bird bgp %v neighbor %v %v\n", b.processName, b.neighborIP, b.neighborAs)
		if b.routeReflector {
			fmt.Fprintf(w, "bird bgp %v rrclient\n", b.processName)
		}
		for net := range b.exportNetworks {
			fmt.Fprintf(w, "bird bgp %v filter %v\n", b.processName, net)
		}
	}

	r.setRouterID()
	fmt.Fprintf(w, "bird routerid %v\n", r.routerID)
	fmt.Fprintf(w, "bird commit\n")

	// ***** firewall stuff ***** //

	fmt.Fprintln(w, "fw flush") // no need to manage firewall state - just start over
	fmt.Fprintf(w, "fw default %s\n", r.FW.defaultAction)

	for name, chain := range r.FW.chains {
		for _, rule := range chain.rules {
			cmd := fmt.Sprintf("fw chain %s action %s", name, rule.action)

			if rule.src != "" {
				cmd = fmt.Sprintf("%s %s", cmd, rule.src)
			}

			cmd = fmt.Sprintf("%s %s %s", cmd, rule.dst, rule.proto)

			fmt.Fprintln(w, cmd)
		}

		fmt.Fprintf(w, "fw chain %s default action %s\n", name, chain.defaultAction)
	}

	for i, rules := range r.FW.rules {
		for _, rule := range rules {
			if rule.src == "" && rule.dst == "" && rule.proto == "" {
				cmd := fmt.Sprintf("fw chain %s apply", rule.action)

				if rule.in {
					cmd = fmt.Sprintf("%s in %d", cmd, i)
				} else {
					cmd = fmt.Sprintf("%s out %d", cmd, i)
				}

				fmt.Fprintln(w, cmd)
				continue
			}

			cmd := fmt.Sprintf("fw %s", rule.action)

			if rule.in {
				cmd = fmt.Sprintf("%s in %d", cmd, i)
			} else {
				cmd = fmt.Sprintf("%s out %d", cmd, i)
			}

			if rule.src != "" {
				cmd = fmt.Sprintf("%s %s", cmd, rule.src)
			}

			cmd = fmt.Sprintf("%s %s %s", cmd, rule.dst, rule.proto)

			fmt.Fprintln(w, cmd)
		}
	}

	return nil
}

// Commit completes the changes to the Minirouter configuartion file
func (r *Router) Commit(ns *Namespace) error {
	log.Debugln("Commit")

	filename := fmt.Sprintf("minirouter-%v", r.vm.GetName())

	// HAX: the default namespace doesn't get a subdir so we should drop the
	// minirouter conf in f_iomBase
	path := *f_iomBase
	if namespace := r.vm.GetNamespace(); namespace != DefaultNamespace {
		path = filepath.Join(*f_iomBase, namespace)
	}

	f, err := os.Create(filepath.Join(path, filename))
	if err != nil {
		return err
	}

	defer f.Close()

	if err := r.writeConfig(f); err != nil {
		return err
	}

	f.Sync()
	r.updateIPs = false // IPs are no longer stale

	// remove any previous commands
	prefix := fmt.Sprintf("minirouter-%v", r.vm.GetName())
	if err := ns.ccServer.DeleteCommands(prefix); err != nil {
		if !strings.HasPrefix(err.Error(), "no such prefix") {
			return err
		}
	}

	filter := &ron.Filter{
		UUID: r.vm.GetUUID(),
	}

	// issue cc commands for this router
	cmd := &ron.Command{
		Command: []string{"rm", filepath.Join("/tmp/miniccc/files", prefix)},
		Prefix:  prefix,
		Filter:  filter,
	}
	ns.ccServer.NewCommand(cmd)

	cmd = &ron.Command{
		Prefix: prefix,
		Filter: filter,
	}
	cmd.FilesSend = append(cmd.FilesSend, prefix)
	ns.ccServer.NewCommand(cmd)

	cmd = &ron.Command{
		Command: []string{"minirouter", "-u", filepath.Join("/tmp/miniccc/files", prefix)},
		Prefix:  prefix,
		Filter:  filter,
	}
	ns.ccServer.NewCommand(cmd)

	return nil
}

// Sets log level for Minirouter
func (r *Router) LogLevel(level string) {
	log.Debug("RouterLogLevel: %v", level)

	r.logLevel = level
}

// Adds an ip address to the specified interface. This could be ethernet or Loopback
func (r *Router) InterfaceAdd(n int, i string, loopback bool) error {
	log.Debug("RouterInterfaceAdd: %v, %v", n, i)
	if loopback {
		if i == "dhcp" {
			return fmt.Errorf("Cannot put %v on loopback", i)
		}
		if !routerIsValidIP(i) {
			return fmt.Errorf("invalid IP: %v", i)
		}
		if r.routerIPExistance(n, i, loopback) {
			return fmt.Errorf("IP %v already exists", i)
		}
		log.Debug("adding Loopback ip %v", i)
		r.Loopbacks[n] = i
		r.updateIPs = true
		return nil
	}
	if n >= len(r.IPs) {
		return fmt.Errorf("no such network index: %v", n)
	}
	if !routerIsValidIP(i) {
		return fmt.Errorf("invalid IP: %v", i)
	}
	if r.routerIPExistance(n, i, loopback) {
		return fmt.Errorf("IP %v already exists", i)
	}
	log.Debug("adding ip %v", i)
	r.IPs[n] = append(r.IPs[n], i)
	r.updateIPs = true

	return nil
}

// Removes ip(s) from interface or loopback
func (r *Router) InterfaceDel(n string, i string, lo bool) error {
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
		r.Loopbacks = make(map[int]string)
		r.updateIPs = true
		return nil
	}
	// check if request is to delete all loopbacks or just specific loopback
	if lo {
		if i == "all" {
			r.Loopbacks = make(map[int]string)
			r.updateIPs = true
			return nil
		}
		delete(r.Loopbacks, network)
		return nil
	}

	if network >= len(r.IPs) {
		return fmt.Errorf("no such network index: %v", network)
	}

	if i == "" || i == "all" {
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

// Checks if the IP is valid
func routerIsValidIP(i string) bool {
	if _, _, err := net.ParseCIDR(i); err != nil && i != "dhcp" {
		return false
	}
	return true
}

// Checks if the IP Currenlty Exists
func (r *Router) routerIPExistance(n int, i string, loopback bool) bool {
	if loopback {
		for _, v := range r.Loopbacks {
			if v == i {
				return true
			}
		}
	} else {
		for _, v := range r.IPs[n] {
			if v == i {
				return true
			}
		}
	}
	return false
}

// Generates router ID based on established rules
// Manually configure has highest preference -> Highest loopback -> Highest interface
func (r *Router) setRouterID() {
	if strings.Compare(r.routerID, "0.0.0.0") != 0 {
		return
	}
	if len(r.Loopbacks) > 0 {
		for _, ip := range r.Loopbacks {
			log.Debug("compare loopback ip %v with current router id %v", ip, r.routerID)
			if bytes.Compare(net.ParseIP(strings.Split(ip, "/")[0]), net.ParseIP(r.routerID)) == 1 {
				log.Debug("found new router id %v", ip)
				r.routerID = strings.Split(ip, "/")[0]
			}
		}
		return
	}
	for _, iplist := range r.IPs {
		for _, ip := range iplist {
			log.Debug("compare int ip %v with current router id %v", ip, r.routerID)
			if bytes.Compare(net.ParseIP(strings.Split(ip, "/")[0]), net.ParseIP(r.routerID)) == 1 {
				log.Debug("found new router id %v", ip)
				r.routerID = strings.Split(ip, "/")[0]
			}
		}
	}
}

// Sets DHCP Range
func (r *Router) DHCPAddRange(addr, low, high string) error {
	d := r.dhcpFindOrCreate(addr)

	d.low = low
	d.high = high

	return nil
}

// Turns on DHCP Function to the router
func (r *Router) DHCPAddRouter(addr, rtr string) error {
	d := r.dhcpFindOrCreate(addr)

	d.router = rtr

	return nil
}

// Adds DNS server infromation to the router for DHCP
func (r *Router) DHCPAddDNS(addr, dns string) error {
	d := r.dhcpFindOrCreate(addr)

	d.dns = dns

	return nil
}

// Creates a static IP reseravtion for DHCP
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

// toString for DHCP object
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
	for k := range d.static {
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
	r.dns[ip] = append(r.dns[ip], hostname)
}

func (r *Router) DNSDel(ip string) error {
	if ip == "" {
		r.dns = make(map[string][]string)
	} else if _, ok := r.dns[ip]; ok {
		delete(r.dns, ip)
	} else {
		return fmt.Errorf("no such ip: %v", ip)
	}
	return nil
}

func (r *Router) Upstream(ip string) {
	r.upstream = ip
}

func (r *Router) UpstreamDel() error {
	r.upstream = ""

	return nil
}

func (r *Router) Gateway(gw string) {
	r.gw = gw
}

func (r *Router) GatewayDel() error {
	r.gw = ""

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

// Adds a static routes for the router
// These can be standard routes or named routes that can referenced by Bird
func (r *Router) RouteStaticAdd(network, nh, name string) {
	if name != "" {
		if r.namedRoutes[name] == nil {
			r.namedRoutes[name] = make(map[string]string)
		}
		if nh == "0" {
			nh = ""
		}
		r.namedRoutes[name][network] = nh
	} else {
		r.staticRoutes[network] = nh
	}
}

// Removes static route(s) for the router
func (r *Router) RouteStaticDel(network string) error {
	if network == "" || network == "all" {
		r.staticRoutes = make(map[string]string)
		return nil
	}

	if _, ok := r.staticRoutes[network]; !ok {
		return fmt.Errorf("no such static route: %v", network)
	}
	delete(r.staticRoutes, network)
	return nil
}

// Removes named route(s) for the router
func (r *Router) NamedRouteStaticDel(network, name string) error {
	if name == "" {
		r.namedRoutes = make(map[string]map[string]string)
		return nil
	}

	if _, ok := r.namedRoutes[name]; !ok {
		return fmt.Errorf("no such named static route: %v", name)
	}

	if network == "all" {
		delete(r.namedRoutes, name)
		return nil
	}

	if _, ok := r.namedRoutes[name][network]; !ok {
		return fmt.Errorf("no such network in named static route: %v", network)
	}

	delete(r.namedRoutes[name], network)
	return nil
}

// Sets up or returns existing OSPF Area
func (r *Router) ospfFindOrCreate(area string) *ospf {
	if o, ok := r.ospfRoutes[area]; ok {
		return o
	}
	o := &ospf{
		area:       area,
		interfaces: make(map[string]map[string]string),
		prefixes:   make(map[string]bool),
	}
	r.ospfRoutes[area] = o
	return o
}

// toString for OSPF objects
func (o *ospf) String() string {
	var out bytes.Buffer

	fmt.Fprintf(&out, "OSPF Area:\t%v\n", o.area)
	fmt.Fprintf(&out, "Interfaces:\n")

	var keys []string
	for k := range o.interfaces {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, iface := range keys {
		fmt.Fprintf(&out, "\t%v\n", iface)
	}

	if len(o.prefixes) > 0 {
		fmt.Fprintln(&out, "OSPF Export Networks or Routes:")

		keys = nil
		for k := range o.prefixes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, filter := range keys {
			fmt.Fprintf(&out, "\t%v\n", filter)
		}
	}

	return out.String()
}

// Adds interface,network and/or filter information for OSPF
func (r *Router) RouteOSPFAdd(area, iface, filter string) {
	o := r.ospfFindOrCreate(area)
	if iface != "" {
		o.interfaces[iface] = make(map[string]string)
	}
	if filter != "" {
		if strings.Contains(filter, "/") {
			o.prefixes[filter] = false
		} else {
			o.prefixes[filter] = true
		}
	}
}

func (r *Router) RouteOSPFOption(area, iface, option, value string) {
	o := r.ospfFindOrCreate(area)
	if iface != "" {
		o.interfaces[iface] = make(map[string]string)
	}

	o.interfaces[iface][option] = value
}

// Deletes an OSPF Interface Setting or the entire area
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
		o.interfaces = make(map[string]map[string]string)
		return nil
	}
	if _, ok := o.interfaces[iface]; ok {
		delete(o.interfaces, iface)
		return nil
	}
	return fmt.Errorf("no such interface: %v", iface)
}

// Deletes a specific network or filter from OSPF
func (r *Router) RouteOSPFDelFilter(area, filter string) error {
	if area == "" {
		r.ospfRoutes = make(map[string]*ospf)
		return nil
	}

	o, ok := r.ospfRoutes[area]
	if !ok {
		return fmt.Errorf("no such area: %v", area)
	}

	if filter == "" {
		o.prefixes = make(map[string]bool)
		return nil
	}

	if _, ok := o.prefixes[filter]; ok {
		delete(o.prefixes, filter)
		return nil
	}

	return fmt.Errorf("no such filter: %v", filter)
}

// Sets up or returns existing BGP Process
func (r *Router) bgpFindOrCreate(bgpprocess string) *bgp {
	log.Debugln("Finding or creating Bgp process")
	if b, ok := r.bgpRoutes[bgpprocess]; ok {
		log.Debug("found bgp %v", b.processName)
		return b
	}
	b := &bgp{
		processName:    bgpprocess,
		exportNetworks: make(map[string]bool),
	}
	log.Debug("created bgp %v", b.processName)
	r.bgpRoutes[bgpprocess] = b
	return b
}

// Adds BGP configuration information
func (r *Router) RouteBGPAdd(islocal bool, processname string, ip string, as int) {
	b := r.bgpFindOrCreate(processname)
	if islocal {
		log.Debugln("Setting local IP and AS bgp")
		b.localIP = ip
		b.localAS = as
	} else {
		log.Debugln("Setting neighbor IP and AS bgp")
		b.neighborIP = ip
		b.neighborAs = as
	}
}

// Adds BGP export rules
func (r *Router) ExportBGP(processname string, all bool, filter string) {
	b := r.bgpFindOrCreate(processname)
	log.Debugln("Setting export")
	if all {
		b.exportNetworks["all"] = true
	} else {
		b.exportNetworks[filter] = true
		//r.RouteStaticAdd(network, "", processname)
	}
}

// Resets BGP route reflector status
func (r *Router) RouteBGPRRDel(processname string) error {
	if _, ok := r.bgpRoutes[processname]; !ok {
		return fmt.Errorf("no such bgp process: %v", processname)
	}
	r.bgpRoutes[processname].routeReflector = false
	return nil
}

// Resets certain BGP settings or deletes the entire bgp process
func (r *Router) RouteBGPDel(processname string, local, clearall bool) error {
	if _, ok := r.bgpRoutes[processname]; !ok && processname != "" {
		return fmt.Errorf("no such bgp process: %v", processname)
	}
	if processname == "" {
		r.bgpRoutes = make(map[string]*bgp)
		return nil
	}
	if !clearall {
		if local {
			r.bgpRoutes[processname].localIP = ""
			r.bgpRoutes[processname].localAS = 0
			return nil
		} else {
			r.bgpRoutes[processname].neighborIP = ""
			r.bgpRoutes[processname].neighborAs = 0
			return nil
		}
	}
	delete(r.bgpRoutes, processname)
	return nil
}

func (r *Router) FirewallDefault(d string) error {
	log.Debug("RouterFirewallDefault: %s", d)

	r.FW.defaultAction = d
	return nil
}

func (r *Router) FirewallAdd(n int, in bool, src, dst, proto, action string) error {
	log.Debug("RouterFirewallAdd: %d, %v, %s, %s, %s, %s", n, in, src, dst, proto, action)

	if n >= len(r.FW.rules) {
		return fmt.Errorf("no such network index: %v", n)
	}

	rule := &fwRule{in: in, src: src, dst: dst, proto: proto, action: action}
	r.FW.rules[n] = append(r.FW.rules[n], rule)

	return nil
}

func (r *Router) FirewallChainDefault(chain, d string) error {
	log.Debug("RouterFirewallChainDefault: %s, %s", chain, d)

	c, ok := r.FW.chains[chain]
	if !ok {
		c = new(fwChain)
		r.FW.chains[chain] = c
	}

	c.defaultAction = d
	return nil
}

func (r *Router) FirewallChainAdd(chain, src, dst, proto, action string) error {
	log.Debug("RouterFirewallChainAdd: %s, %s, %s, %s, %s", chain, src, dst, proto, action)

	c, ok := r.FW.chains[chain]
	if !ok {
		c = &fwChain{defaultAction: "drop"}
		r.FW.chains[chain] = c
	}

	rule := &fwRule{src: src, dst: dst, proto: proto, action: action}
	c.rules = append(c.rules, rule)

	return nil
}

func (r *Router) FirewallChainApply(n int, in bool, chain string) error {
	log.Debug("RouterFirewallChainApply: %d, %v, %s", n, in, chain)

	if _, ok := r.FW.chains[chain]; !ok {
		return fmt.Errorf("unknown chain %s", chain)
	}

	if n >= len(r.FW.rules) {
		return fmt.Errorf("no such network index: %v", n)
	}

	rule := &fwRule{in: in, action: chain}
	r.FW.rules[n] = append(r.FW.rules[n], rule)

	return nil
}

func (r *Router) FirewallFlush() error {
	log.Debug("RouterFirewallFlush")

	i := len(r.FW.rules)
	r.FW = &fw{
		defaultAction: "accept",
		rules:         make([][]*fwRule, i),
		chains:        make(map[string]*fwChain),
	}

	return nil
}

// toString for BGP Object
func (b *bgp) String() string {
	var out bytes.Buffer
	fmt.Fprintf(&out, "BGP Process Name:\t%v\n", b.processName)
	fmt.Fprintf(&out, "BGP Local IP:\t%v\n", b.localIP)
	fmt.Fprintf(&out, "BGP Local As:\t%v\n", b.localAS)
	fmt.Fprintf(&out, "BGP Neighbor IP:\t%v\n", b.neighborIP)
	fmt.Fprintf(&out, "BGP Neighbor As:\t%v\n", b.neighborAs)
	fmt.Fprintf(&out, "BGP RouteReflector:\t%v\n", b.routeReflector)
	if len(b.exportNetworks) > 0 {
		fmt.Fprintln(&out, "BGP Export Networks or Routes:")

		var keys []string
		for k := range b.exportNetworks {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, filter := range keys {
			fmt.Fprintf(&out, "\t%v\n", filter)
		}
	}
	return out.String()
}
