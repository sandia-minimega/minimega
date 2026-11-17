package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestGenerateConfig(t *testing.T) {
	r := NewRouter(3)
	r.InterfaceAdd(0, "192.168.1.1/24", false)
	r.InterfaceAdd(1, "192.168.2.1/24", false)
	r.InterfaceAdd(2, "192.168.3.1/24", false)
	r.InterfaceAdd(4, "192.168.4.1/32", true)
	r.RouteStaticAdd("0.0.0.0/0", "192.168.1.2", "defaultroute")
	r.RouteStaticAdd("192.168.1.0/24", "172.16.1.1", "route1")
	r.RouteStaticAdd("192.168.2.0/24", "0", "route2")
	r.RouteOSPFAdd("0", "0", "")
	r.RouteOSPFAdd("0", "1", "")
	r.RouteOSPFAdd("0", "", "192.168.3.0/24")
	r.RouteOSPFAdd("0", "", "defaultroute")
	r.RouteBGPAdd(false, "r2", "192.168.1.1", 200)
	r.RouteBGPAdd(true, "r2", "192.168.1.2", 100)
	r.ExportBGP("r2", true, "")
	r.ExportBGP("r2", false, "route1")
	r.ExportBGP("r2", false, "route2")

	var b bytes.Buffer
	r.writeConfig(&b)
	configtest(b.String(), testGenerateConfigWant, t)
}

func configtest(test, control string, t *testing.T) {
	want := map[string]bool{}
	for _, v := range strings.Split(control, "\n") {
		want[v] = true
	}

	// delete lines in both
	got := map[string]bool{}
	for _, v := range strings.Split(test, "\n") {
		if _, ok := want[v]; !ok && v != "" {
			got[v] = true
		} else {
			delete(want, v)
		}
	}

	for v := range want {
		t.Error("- ", v)
	}
	for v := range got {
		t.Error("+ ", v)
	}
}

func TestInterfaceAdd(t *testing.T) {
	r := NewRouter(3)
	//Positive Testing
	if err := r.InterfaceAdd(0, "192.168.1.1/24", false); err != nil {
		t.Error(err)
	}
	if err := r.InterfaceAdd(1, "192.168.2.1/24", false); err != nil {
		t.Error(err)
	}
	if err := r.InterfaceAdd(2, "192.168.3.1/24", false); err != nil {
		t.Error(err)
	}
	if err := r.InterfaceAdd(4, "192.168.4.1/32", true); err != nil {
		t.Error(err)
	}
	//Negative Testing
	if err := r.InterfaceAdd(3, "192.168.4.1/32", false); err.Error() != fmt.Errorf("no such network index: %v", 3).Error() {
		t.Error("Expected error no such network index: but got ", err)
	}
	if err := r.InterfaceAdd(0, "192.168.1.1/24", false); err.Error() != fmt.Errorf("IP %v already exists", "192.168.1.1/24").Error() {
		t.Error("Expected error ip already exist: but got ", err)
	}
	if err := r.InterfaceAdd(4, "192.168.4", true); err.Error() != fmt.Errorf("invalid IP: %v", "192.168.4").Error() {
		t.Error("Expected error invalid IP: but got ", err)
	}
	configtest(r.String(), testInterfaceWant, t)
}

func TestRouteOSPFAdd(t *testing.T) {
	r := NewRouter(3)
	r.RouteOSPFAdd("0", "0", "")
	r.RouteOSPFAdd("0", "", "192.168.3.0/24")
	o, ok := r.ospfRoutes["0"]
	if ok {
		if _, ok := o.interfaces["0"]; !ok {
			t.Error("unable to find interface")
		} else if _, ok := o.prefixes["192.168.3.0/24"]; !ok {
			t.Error("unable to find filter")
		}
	} else {
		t.Error("unable to find area")
	}
	configtest(r.String(), testOSPFWant, t)
}

func TestRouteStaticAdd(t *testing.T) {
	r := NewRouter(3)
	r.RouteStaticAdd("0.0.0.0/0", "192.168.1.2", "defaultroute")
	r.RouteStaticAdd("192.168.1.1/32", "0", "ospf")
	r.RouteStaticAdd("192.168.1.2/32", "", "ospf")
	r.RouteStaticAdd("172.16.1.1/24", "192.168.1.2", "")
	if _, ok := r.staticRoutes["172.16.1.1/24"]; !ok {
		t.Error("unable to find static route")
	}
	if _, ok := r.namedRoutes["ospf"]; !ok {
		t.Error("unable to find named route:ospf")
	}
	if _, ok := r.namedRoutes["ospf"]["192.168.1.1/32"]; !ok {
		t.Error("unable to find named route:ospf:192.168.1.1/32")
	}
	if _, ok := r.namedRoutes["ospf"]["192.168.1.2/32"]; !ok {
		t.Error("unable to find named route:ospf:192.168.1.2/32")
	}
	if _, ok := r.namedRoutes["defaultroute"]; !ok {
		t.Error("unable to find named route:defaultroute")
	}
	configtest(r.String(), testStaticWant, t)
}

func TestRouteBGPAdd(t *testing.T) {
	r := NewRouter(3)
	r.RouteBGPAdd(false, "r1", "192.168.1.1", 100)
	r.RouteBGPAdd(true, "r1", "192.168.1.2", 200)
	if _, ok := r.bgpRoutes["r1"]; !ok {
		t.Error("unable to find bgp route")
	}
	if r.bgpRoutes["r1"].localIP != "192.168.1.2" || r.bgpRoutes["r1"].localAS != 200 {
		t.Error("localip/as incorrect match")
	}
	if r.bgpRoutes["r1"].neighborIP != "192.168.1.1" || r.bgpRoutes["r1"].neighborAs != 100 {
		t.Error("neighbor ip/as incorrect match")
	}
	configtest(r.String(), testBGPAddWant, t)
}

func TestExportBGP(t *testing.T) {
	r := NewRouter(3)
	r.ExportBGP("r1", true, "")
	r.ExportBGP("r1", false, "r1route")
	if _, ok := r.bgpRoutes["r1"].exportNetworks["all"]; !ok {
		t.Error("unable to find test export statement: all")
	}
	if _, ok := r.bgpRoutes["r1"].exportNetworks["r1route"]; !ok {
		t.Error("unable to find test export statement: r1")
	}
	configtest(r.String(), testBGPExportWant, t)
}

const testGenerateConfigWant = `
log level error
ip flush
ip add 0 192.168.1.1/24
ip add 1 192.168.2.1/24
ip add 2 192.168.3.1/24
ip add lo 192.168.4.1/32
dnsmasq flush
route del default
dnsmasq commit
bird flush
bird static 0.0.0.0/0 192.168.1.2 defaultroute
bird static 192.168.1.0/24 172.16.1.1 route1
bird static 192.168.2.0/24 null route2
bird ospf 0 0
bird ospf 0 1
bird ospf 0 filter 192.168.3.0/24
bird ospf 0 filter defaultroute
bird bgp r2 local 192.168.1.2 100
bird bgp r2 neighbor 192.168.1.1 200
bird bgp r2 filter all
bird bgp r2 filter route1
bird bgp r2 filter route2
bird routerid 192.168.4.1
bird commit
`
const testInterfaceWant = `
IPs:
Network: 0: [192.168.1.1/24]
Network: 1: [192.168.2.1/24]
Network: 2: [192.168.3.1/24]
Loopback IPs:
192.168.4.1/32
Firewall Rules:
  Default Action: accept
`
const testOSPFWant = `
IPs:
Network: 0: []
Network: 1: []
Network: 2: []
OSPF Area:	0
Interfaces:
	0
OSPF Export Networks or Routes:
	192.168.3.0/24
Firewall Rules:
  Default Action: accept
`
const testStaticWant = `
IPs:
Network: 0: []
Network: 1: []
Network: 2: []
Static Routes:
172.16.1.1/24	192.168.1.2
Named Static Routes:
defaultroute
	0.0.0.0/0	192.168.1.2
ospf
	192.168.1.1/32
	192.168.1.2/32
Firewall Rules:
  Default Action: accept
`
const testBGPAddWant = `
IPs:
Network: 0: []
Network: 1: []
Network: 2: []
BGP Process Name:	r1
BGP Local IP:	192.168.1.2
BGP Local As:	200
BGP Neighbor IP:	192.168.1.1
BGP Neighbor As:	100
BGP RouteReflector:	false
Firewall Rules:
  Default Action: accept
`
const testBGPExportWant = `
IPs:
Network: 0: []
Network: 1: []
Network: 2: []
BGP Process Name:	r1
BGP Local IP:	
BGP Local As:	0
BGP Neighbor IP:	
BGP Neighbor As:	0
BGP RouteReflector:	false
BGP Export Networks or Routes:
	all
	r1route
Firewall Rules:
  Default Action: accept
`

// A negative network index (from the CLI, e.g. "router foo fw accept in -1 ...")
// must return an error rather than panic minimega.
func TestNegativeNetworkIndex(t *testing.T) {
	r := NewRouter(2)
	r.FirewallChainAdd("c", "any", "any", "tcp", "accept")

	calls := map[string]func() error{
		"InterfaceAdd":       func() error { return r.InterfaceAdd(-1, "10.0.0.1/24", false) },
		"InterfaceDel":       func() error { return r.InterfaceDel("-2", "all", false) },
		"FirewallAdd":        func() error { return r.FirewallAdd(-1, true, "10.0.0.1", "any", "tcp", "accept") },
		"FirewallChainApply": func() error { return r.FirewallChainApply(-1, true, "c") },
	}
	for name, f := range calls {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("%s panicked: %v", name, p)
				}
			}()
			if err := f(); err == nil {
				t.Errorf("%s: expected error for negative index", name)
			}
		}()
	}
}

// "route ospf <area> <net> <option> <value>" calls RouteOSPFAdd then
// RouteOSPFOption; later commands must not discard earlier options.
func TestRouteOSPFOptionsAccumulate(t *testing.T) {
	r := NewRouter(2)
	for _, kv := range [][2]string{{"cost", "10"}, {"hello", "5"}} {
		r.RouteOSPFAdd("0", "1", "")
		r.RouteOSPFOption("0", "1", kv[0], kv[1])
	}
	r.RouteOSPFAdd("0", "1", "") // re-adding the interface keeps options

	got := r.ospfRoutes["0"].interfaces["1"]
	if got["cost"] != "10" || got["hello"] != "5" {
		t.Errorf("options lost, got %v", got)
	}
}

// Clearing a single loopback after a commit must make the next config
// flush the IPs, otherwise the old loopback stays on the running router.
func TestInterfaceDelLoopbackFlushesIPs(t *testing.T) {
	r := NewRouter(3)
	if err := r.InterfaceAdd(2, "11.0.0.1/32", true); err != nil {
		t.Fatal(err)
	}
	r.updateIPs = false // simulate a commit

	if err := r.InterfaceDel("2", "", true); err != nil {
		t.Fatal(err)
	}

	var b bytes.Buffer
	r.writeConfig(&b)
	if !strings.Contains(b.String(), "ip flush") {
		t.Errorf("config does not flush IPs after loopback removal:\n%s", b.String())
	}
}

// An auto-selected router ID must follow later address changes; only a
// manually configured ID is sticky.
func TestRouterIDRecomputed(t *testing.T) {
	r := NewRouter(2)
	rid := func() string {
		var b bytes.Buffer
		r.writeConfig(&b)
		for _, l := range strings.Split(b.String(), "\n") {
			if strings.HasPrefix(l, "bird routerid ") {
				return strings.TrimPrefix(l, "bird routerid ")
			}
		}
		return ""
	}

	r.InterfaceAdd(0, "10.0.0.1/24", false)
	if got := rid(); got != "10.0.0.1" {
		t.Fatalf("first commit: got %q", got)
	}

	r.InterfaceDel("0", "all", false)
	r.InterfaceAdd(0, "172.16.0.1/24", false)
	if got := rid(); got != "172.16.0.1" {
		t.Errorf("after address change: got %q, want 172.16.0.1", got)
	}

	r.InterfaceAdd(1, "1.1.1.1/32", true)
	if got := rid(); got != "1.1.1.1" {
		t.Errorf("after adding loopback: got %q, want 1.1.1.1", got)
	}

	r.routerID = "9.9.9.9" // manual
	if got := rid(); got != "9.9.9.9" {
		t.Errorf("manual id: got %q", got)
	}
}

// "router vm route bgp p local <ip> <asnumber>" with a non-numeric AS must
// return an error and leave the BGP process untouched.
func TestCLIRouterBGPInvalidAS(t *testing.T) {
	vm := &KvmVM{BaseVM: &BaseVM{Name: "vm0"}}
	ns := &Namespace{VMs: VMs{m: map[int]VM{0: vm}}, routers: map[int]*Router{}}
	r := NewRouter(1)
	r.vm = vm
	ns.routers[vm.GetID()] = r

	run := func(as string) error {
		c := &minicli.Command{
			StringArgs: map[string]string{"vm": "vm0", "processname": "bar", "IPv4": "10.0.0.1", "asnumber": as},
			BoolArgs:   map[string]bool{"route": true, "bgp": true, "local": true},
		}
		return cliRouter(ns, c, &minicli.Response{})
	}

	if err := run("AS100"); err == nil {
		t.Error("expected error for non-numeric AS number")
	}
	if b := r.bgpRoutes["bar"]; b != nil {
		t.Errorf("invalid AS created bgp process: %v", b)
	}
	if err := run("100"); err != nil {
		t.Fatal(err)
	}
	if got := r.bgpRoutes["bar"].localAS; got != 100 {
		t.Errorf("localAS = %v, want 100", got)
	}
}
