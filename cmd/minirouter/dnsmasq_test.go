// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestDnsmasqCustom(t *testing.T) {
	defer func() {
		dnsmasqData = &Dnsmasq{DHCP: map[string]*Dhcp{}, DNS: map[string][]string{}, RAD: map[string]bool{}}
	}()

	// lines as minimega's router writeConfig quotes them
	for _, cmd := range []string{
		"dnsmasq flush",
		`dnsmasq custom "address=/test.com/1.0.0.1"`,
		`dnsmasq custom "txt-record=a.live,\"x # y \\ z\""`,
	} {
		r, err := minicli.ProcessString(cmd, false)
		if err != nil {
			t.Fatalf("%v: %v", cmd, err)
		}
		for range r {
		}
	}

	want := []string{"address=/test.com/1.0.0.1", `txt-record=a.live,"x # y \ z"`}
	if strings.Join(dnsmasqData.Custom, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", dnsmasqData.Custom, want)
	}

	var b bytes.Buffer
	if err := template.Must(template.New("dnsmasq").Parse(dnsmasqTmpl)).Execute(&b, dnsmasqData); err != nil {
		t.Fatal(err)
	}
	for _, line := range want {
		if !strings.Contains(b.String(), "\n"+line+"\n") {
			t.Errorf("config missing %q:\n%s", line, b.String())
		}
	}

	r, _ := minicli.ProcessString("dnsmasq flush", false)
	for range r {
	}
	if len(dnsmasqData.Custom) != 0 {
		t.Errorf("flush kept custom lines: %q", dnsmasqData.Custom)
	}
}
