package main

import (
	"bytes"
	"fmt"
	//"math/rand"
	//"strconv"
	"strings"
	"testing"
)

func generaterouter() *Router {
	r := &Router{
		IPs:          [][]string{},
		Loopbacks:    make(map[int]string),
		logLevel:     "error",
		dhcp:         make(map[string]*dhcp),
		dns:          make(map[string][]string),
		rad:          make(map[string]bool),
		staticRoutes: make(map[string]string),
		namedRoutes:  make(map[string]map[string]string),
		ospfRoutes:   make(map[string]*ospf),
		bgpRoutes:    make(map[string]*bgp),
		routerid:     "0.0.0.0",
	}
	for i := 0; i < 3; i++ {
		r.IPs = append(r.IPs, []string{})
	}
	return r
}

func TestGenerateConfig(t *testing.T) {
	r := generaterouter()
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

	if ok, err := configmatch(b.String(), 0); !ok {
		t.Error("Mismatch Generating bird config:", err)
	}
}

func configmatch(test string, t int) (bool, string) {
	var control map[string]bool
	testarr := strings.Split(test, "\n")
	switch t {
	case 0:
		control = map[string]bool{
			"log level error":          true,
			"ip flush":                 true,
			"ip add 0 192.168.1.1/24":  true,
			"ip add 1 192.168.2.1/24":  true,
			"ip add 2 192.168.3.1/24":  true,
			"ip add lo 192.168.4.1/32": true,
			"dnsmasq flush":            true,
			"route del default":        true,
			"dnsmasq commit":           true,
			"bird flush":               true,
			"bird static 0.0.0.0/0 192.168.1.2 defaultroute": true,
			"bird static 192.168.1.0/24 172.16.1.1 route1":   true,
			"bird static 192.168.2.0/24 null route2":         true,
			"bird ospf 0 0":                                  true,
			"bird ospf 0 1":                                  true,
			"bird ospf 0 filter 192.168.3.0/24":              true,
			"bird ospf 0 filter defaultroute":                true,
			"bird bgp r2 local 192.168.1.2 100":              true,
			"bird bgp r2 neighbor 192.168.1.1 200":           true,
			"bird bgp r2 filter all":                         true,
			"bird bgp r2 filter route1":                      true,
			"bird bgp r2 filter route2":                      true,
			"bird routerid 192.168.4.1":                      true,
			"bird commit":                                    true,
			"":                                               true,
		}
		if len(testarr) != len(control) {
			return false, "length mismatch"
		}
	case 1:
		control = map[string]bool{
			"IPs:":                         true,
			"Network: 0: [192.168.1.1/24]": true,
			"Network: 1: [192.168.2.1/24]": true,
			"Network: 2: [192.168.3.1/24]": true,
			"Loopback IPs:":                true,
			"192.168.4.1/32":               true,
			"":                             true,
		}
	case 2:
		control = map[string]bool{
			"IPs:":                            true,
			"Network: 0: []":                  true,
			"Network: 1: []":                  true,
			"Network: 2: []":                  true,
			"OSPF Area:\t0":                   true,
			"Interfaces:":                     true,
			"\t0":                             true,
			"OSPF Export Networks or Routes:": true,
			"\t192.168.3.0/24":                true,
			"":                                true,
		}
	case 3:
		control = map[string]bool{
			"IPs:":           true,
			"Network: 0: []": true,
			"Network: 1: []": true,
			"Network: 2: []": true,
			"Static Routes:": true,
			"172.16.1.1/24	192.168.1.2": true,
			"Named Static Routes:":     true,
			"defaultroute":             true,
			"\t0.0.0.0/0\t192.168.1.2": true,
			"ospf":                     true,
			"\t192.168.1.1/32":         true,
			"\t192.168.1.2/32":         true,
			"":                         true,
		}
	case 4:
		control = map[string]bool{
			"IPs:":                          true,
			"Network: 0: []":                true,
			"Network: 1: []":                true,
			"Network: 2: []":                true,
			"BGP Process Name:\tr1":         true,
			"BGP Local IP:\t192.168.1.2":    true,
			"BGP Local As:\t200":            true,
			"BGP Neighbor IP:\t192.168.1.1": true,
			"BGP Neighbor As:\t100":         true,
			"BGP RouteReflector:\tfalse":    true,
			"":                              true,
		}
	case 5:
		control = map[string]bool{
			"IPs:":                           true,
			"Network: 0: []":                 true,
			"Network: 1: []":                 true,
			"Network: 2: []":                 true,
			"BGP Process Name:\tr1":          true,
			"BGP Local IP:\t":                true,
			"BGP Local As:\t0":               true,
			"BGP Neighbor IP:\t":             true,
			"BGP Neighbor As:\t0":            true,
			"BGP RouteReflector:\tfalse":     true,
			"BGP Export Networks or Routes:": true,
			"\tall":                          true,
			"\tr1route":                      true,
			"":                               true,
		}
	}

	for _, c := range testarr {
		if _, ok := control[c]; !ok {
			return false, c
		}
	}

	return true, ""
}

func TestInterfaceAdd(t *testing.T) {
	r := generaterouter()
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
	if ok, err := configmatch(r.String(), 1); !ok {
		t.Error("To String mismatch:", err)
	}
}

func TestRouteOSPFAdd(t *testing.T) {
	r := generaterouter()
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
	if ok, err := configmatch(r.String(), 2); !ok {
		t.Error("To String mismatch:", err)
	}
}

func TestRouteStaticAdd(t *testing.T) {
	r := generaterouter()
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
	if ok, err := configmatch(r.String(), 3); !ok {
		t.Error("To String mismatch:", err)
	}
}

func TestRouteBGPAdd(t *testing.T) {
	r := generaterouter()
	r.RouteBGPAdd(false, "r1", "192.168.1.1", 100)
	r.RouteBGPAdd(true, "r1", "192.168.1.2", 200)
	if _, ok := r.bgpRoutes["r1"]; !ok {
		t.Error("unable to find bgp route")
	}
	if r.bgpRoutes["r1"].localip != "192.168.1.2" || r.bgpRoutes["r1"].localas != 200 {
		t.Error("localip/as incorrect match")
	}
	if r.bgpRoutes["r1"].neighborip != "192.168.1.1" || r.bgpRoutes["r1"].neighboras != 100 {
		t.Error("neighbor ip/as incorrect match")
	}
	if ok, err := configmatch(r.String(), 4); !ok {
		t.Error("To String mismatch:", err)
	}
}

func TestExportBGP(t *testing.T) {
	r := generaterouter()
	r.ExportBGP("r1", true, "")
	r.ExportBGP("r1", false, "r1route")
	if _, ok := r.bgpRoutes["r1"].exportnetworks["all"]; !ok {
		t.Error("unable to find test export statement: all")
	}
	if _, ok := r.bgpRoutes["r1"].exportnetworks["r1route"]; !ok {
		t.Error("unable to find test export statement: r1")
	}
	if ok, err := configmatch(r.String(), 5); !ok {
		t.Error("To String mismatch:", err)
	}
}
