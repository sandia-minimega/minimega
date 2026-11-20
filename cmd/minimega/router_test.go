package main

import (
	"bufio"
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

func TestDnsmasqCustom(t *testing.T) {
	r := NewRouter(1)
	for _, line := range []string{
		"address=/test.com/1.0.0.1",
		"  server=/test2.com/3.0.0.1  ",
		"address=/test.com/1.0.0.1", // duplicate, ignored
		`txt-record=a.live,"x # y \ z"`,
	} {
		if err := r.DnsmasqAdd(line); err != nil {
			t.Fatalf("DnsmasqAdd(%q): %v", line, err)
		}
	}
	for _, line := range []string{"", "   ", "a=1\nb=2", "a=1\r", "a\x00"} {
		if err := r.DnsmasqAdd(line); err == nil {
			t.Errorf("DnsmasqAdd(%q) should fail", line)
		}
	}

	want := []string{"address=/test.com/1.0.0.1", "server=/test2.com/3.0.0.1", `txt-record=a.live,"x # y \ z"`}
	if strings.Join(r.dnsmasq, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", r.dnsmasq, want)
	}

	var b bytes.Buffer
	r.writeConfig(&b)
	for _, line := range []string{
		`dnsmasq custom "address=/test.com/1.0.0.1"`,
		`dnsmasq custom "server=/test2.com/3.0.0.1"`,
		`dnsmasq custom "txt-record=a.live,\"x # y \\ z\""`,
	} {
		if !strings.Contains(b.String(), line+"\n") {
			t.Errorf("config missing %q:\n%s", line, b.String())
		}
	}
	// custom lines must reach minirouter before dnsmasq is restarted
	if strings.Index(b.String(), "dnsmasq custom") > strings.Index(b.String(), "dnsmasq commit") {
		t.Errorf("custom lines written after commit:\n%s", b.String())
	}

	if !strings.Contains(r.String(), "Custom dnsmasq config:\naddress=/test.com/1.0.0.1\n") {
		t.Errorf("String() missing custom config:\n%s", r.String())
	}

	if err := r.DnsmasqDel("server=/test2.com/3.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := r.DnsmasqDel("server=/test2.com/3.0.0.1"); err == nil {
		t.Error("deleting a missing line should fail")
	}
	if len(r.dnsmasq) != 2 || r.dnsmasq[0] != "address=/test.com/1.0.0.1" {
		t.Errorf("unexpected lines after delete: %q", r.dnsmasq)
	}
	if err := r.DnsmasqDel(""); err != nil || len(r.dnsmasq) != 0 {
		t.Errorf("clearing all lines: err=%v, left %q", err, r.dnsmasq)
	}
	b.Reset()
	r.writeConfig(&b)
	if strings.Contains(b.String(), "dnsmasq custom") {
		t.Errorf("cleared lines still written:\n%s", b.String())
	}
}

func TestClearRouterDnsmasq(t *testing.T) {
	vm := &KvmVM{BaseVM: &BaseVM{ID: 1, Name: "rtr", BaseConfig: BaseConfig{Tags: map[string]string{}}}}
	ns := &Namespace{Name: "test", VMs: VMs{m: map[int]VM{1: vm}}, routers: map[int]*Router{}}

	run := func(fn func(*Namespace, *minicli.Command, *minicli.Response) error, input string) {
		t.Helper()
		if err := fn(ns, minicli.MustCompile(input), &minicli.Response{}); err != nil {
			t.Fatalf("%v: %v", input, err)
		}
	}

	run(cliRouter, "router rtr dnsmasq address=/test.com/1.0.0.1")
	run(cliRouter, "router rtr dnsmasq server=/test2.com/3.0.0.1")
	run(cliClearRouter, "clear router rtr dnsmasq server=/test2.com/3.0.0.1")
	if got := ns.routers[1].dnsmasq; len(got) != 1 || got[0] != "address=/test.com/1.0.0.1" {
		t.Fatalf("after clearing one line: %q", got)
	}

	// clearing the whole router must drop custom lines too
	run(cliClearRouter, "clear router rtr")
	if got := ns.routers[1].dnsmasq; len(got) != 0 {
		t.Fatalf("clear router left custom lines: %q", got)
	}
}

func TestDnsmasqAddTooLong(t *testing.T) {
	r := NewRouter(0)
	max := bufio.MaxScanTokenSize - 1 - len(dnsmasqCustomCmd(""))
	if err := r.DnsmasqAdd(strings.Repeat("a", max)); err != nil {
		t.Errorf("line at the limit rejected: %v", err)
	}
	if err := r.DnsmasqAdd(strings.Repeat("b", max+1)); err == nil {
		t.Error("line over the limit accepted")
	}
	// quoting escapes count toward the limit
	if err := r.DnsmasqAdd(strings.Repeat(`"`, max/2+1)); err == nil {
		t.Error("escaped line over the limit accepted")
	}
}
