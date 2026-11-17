// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

func TestDnsmasqTemplateRelay(t *testing.T) {
	data := &Dnsmasq{
		DHCP: map[string]*Dhcp{
			"10.0.1.1": {Addr: "10.0.1.1", Relay: []string{"10.0.0.2", "10.0.0.3"}, Static: map[string]string{}},
			"10.0.2.1": {Addr: "10.0.2.1", Low: "10.0.2.100", High: "10.0.2.200", Netmask: "255.255.255.0", Static: map[string]string{}},
			"10.0.3.1": {Addr: "10.0.3.1", Netmask: "255.255.255.0", Static: map[string]string{"00:11:22:33:44:55": "10.0.3.10"}},
			"10.0.4.1": {Addr: "10.0.4.1", Low: "10.0.4.100", High: "10.0.4.200", Static: map[string]string{}},
		},
		DNS: map[string][]string{},
		RAD: map[string]bool{},
	}

	var b bytes.Buffer
	if err := template.Must(template.New("dnsmasq").Parse(dnsmasqTmpl)).Execute(&b, data); err != nil {
		t.Fatal(err)
	}
	got := b.String()

	for _, want := range []string{
		"dhcp-relay=10.0.1.1,10.0.0.2\n",
		"dhcp-relay=10.0.1.1,10.0.0.3\n",
		"dhcp-range=set:10.0.2.1,10.0.2.100,10.0.2.200,255.255.255.0\n",
		"dhcp-range=set:10.0.3.1,10.0.3.1,static,255.255.255.0\n",
		"dhcp-range=set:10.0.4.1,10.0.4.100,10.0.4.200\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("config missing %q:\n%s", want, got)
		}
	}
	// a relayed address must not also be served
	if strings.Contains(got, "set:10.0.1.1") {
		t.Errorf("relayed address also has a DHCP range:\n%s", got)
	}
}
