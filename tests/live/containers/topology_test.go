// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build live

package containers

// vmSpec is what topology.mm is expected to have produced for one VM.
type vmSpec struct {
	Name      string
	Container bool     // a container, not a KVM VM
	Hostname  string   // what the guest reports
	VLANs     []string // VLAN aliases, in NIC order
	IPs       []string // expected IPv4 per NIC; "" means any address in Subnet
	Subnet    string   // prefix for dynamic addresses ("10.20.1.")
	Memory    int      // MB
	VCPUs     int
	Init      string // argument of the init program
	Tags      map[string]string
}

// The topology defined in topology.mm. Keep the two in sync.
var topology = []vmSpec{
	{Name: "ctr-rtr", Container: true, Hostname: "ctr-rtr", VLANs: []string{"lan-a", "lan-b"}, IPs: []string{"10.20.1.1", "10.20.2.1"},
		Memory: 256, VCPUs: 1, Init: "router", Tags: map[string]string{"role": "router"}},
	{Name: "ctr-a1", Container: true, Hostname: "alpha", VLANs: []string{"lan-a"}, IPs: []string{""}, Subnet: "10.20.1.",
		Memory: 192, VCPUs: 2, Init: "host", Tags: map[string]string{"role": "client", "lan": "a"}},
	{Name: "ctr-a2", Container: true, Hostname: "ctr-a2", VLANs: []string{"lan-a"}, IPs: []string{""}, Subnet: "10.20.1.",
		Memory: 128, VCPUs: 1, Init: "host", Tags: map[string]string{"role": "client", "lan": "a"}},
	{Name: "ctr-a3", Container: true, Hostname: "ctr-a3", VLANs: []string{"lan-a"}, IPs: []string{""}, Subnet: "10.20.1.",
		Memory: 128, VCPUs: 1, Init: "host", Tags: map[string]string{"role": "client", "lan": "a"}},
	{Name: "ctr-b1", Container: true, Hostname: "ctr-b1", VLANs: []string{"lan-b"}, IPs: []string{"10.20.2.11"},
		Memory: 128, VCPUs: 1, Init: "host", Tags: map[string]string{"role": "server", "lan": "b"}},
	{Name: "kvm-b1", Hostname: "kvm-b1", VLANs: []string{"lan-b"}, IPs: []string{"10.20.2.21"},
		Memory: 384, VCPUs: 1, Tags: map[string]string{"role": "server", "lan": "b"}},
}

const (
	a1UUID = "4c1e0000-0000-4000-8000-0000000000a1"
	// total NICs across the topology (router 2, hosts 5)
	totalNICs = 7
)

func specFor(name string) vmSpec {
	for _, s := range topology {
		if s.Name == name {
			return s
		}
	}
	return vmSpec{}
}

func containerVMs() []vmSpec {
	var out []vmSpec
	for _, s := range topology {
		if s.Container {
			out = append(out, s)
		}
	}
	return out
}
