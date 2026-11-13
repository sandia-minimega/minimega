// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build live

package core

// vmSpec is what topology.mm is expected to have produced for one VM.
type vmSpec struct {
	Name   string
	VLANs  []string // VLAN aliases, in NIC order
	IPs    []string // expected IPv4 per NIC; "" means any address in Subnet
	Subnet string   // prefix for dynamic addresses ("10.10.1.")
	Memory string
	VCPUs  string
	CC     bool // runs miniccc over a backchannel
	Router bool
	Tags   map[string]string
}

// hostname is what the guest reports. host-a2..a5 are launched as a range
// from one configuration, so they keep the image's default hostname.
func (s vmSpec) hostname() string {
	switch s.Name {
	case "host-a2", "host-a3", "host-a4", "host-a5":
		return "live-host"
	}
	return s.Name
}

// The topology defined in topology.mm. Keep the two in sync.
var topology = func() []vmSpec {
	var vms []vmSpec

	vms = append(vms,
		vmSpec{Name: "rtr-ab", VLANs: []string{"lan-a", "lan-b"}, IPs: []string{"10.10.1.1", "10.10.2.1"},
			Memory: "448", VCPUs: "1", CC: true, Router: true, Tags: map[string]string{"role": "router"}},
		vmSpec{Name: "rtr-cd", VLANs: []string{"lan-b", "lan-c", "lan-d"}, IPs: []string{"10.10.2.2", "10.10.3.1", "10.10.4.1"},
			Memory: "448", VCPUs: "1", CC: true, Router: true, Tags: map[string]string{"role": "router"}},
	)

	for i := 1; i <= 5; i++ {
		vms = append(vms, vmSpec{
			Name: "host-a" + itoa(i), VLANs: []string{"lan-a"}, IPs: []string{""}, Subnet: "10.10.1.",
			Memory: "384", VCPUs: "1", CC: true, Tags: map[string]string{"role": "client", "lan": "a"},
		})
	}
	for i := 1; i <= 5; i++ {
		vcpus := "1"
		if i == 3 {
			vcpus = "2"
		}
		vms = append(vms, vmSpec{
			Name: "host-b" + itoa(i), VLANs: []string{"lan-b"}, IPs: []string{"10.10.2.1" + itoa(i)},
			Memory: "352", VCPUs: vcpus, CC: true, Tags: map[string]string{"role": "client", "lan": "b"},
		})
	}
	for i := 1; i <= 5; i++ {
		vms = append(vms, vmSpec{
			Name: "host-c" + itoa(i), VLANs: []string{"lan-c"}, IPs: []string{"10.10.3.1" + itoa(i)},
			Memory: "384", VCPUs: "1", CC: true, Tags: map[string]string{"role": "server", "lan": "c"},
		})
	}
	for i := 1; i <= 4; i++ {
		vms = append(vms, vmSpec{
			Name: "host-d" + itoa(i), VLANs: []string{"lan-d"}, IPs: []string{"10.10.4.1" + itoa(i)},
			Memory: "384", VCPUs: "1", CC: i != 4, Tags: map[string]string{"role": "server", "lan": "d"},
		})
	}
	vms = append(vms, vmSpec{
		Name: "host-d5", VLANs: []string{"lan-d", "lan-c"}, IPs: []string{"10.10.4.15", "10.10.3.16"},
		Memory: "384", VCPUs: "1", CC: true, Tags: map[string]string{"role": "server", "lan": "d"},
	})

	return vms
}()

const (
	d5UUID = "4c1e0000-0000-4000-8000-0000000000d5"
	// total NICs across the topology (routers 5, hosts 21)
	totalNICs = 26
)

func specFor(name string) vmSpec {
	for _, s := range topology {
		if s.Name == name {
			return s
		}
	}
	return vmSpec{}
}

func ccVMs() []vmSpec {
	var out []vmSpec
	for _, s := range topology {
		if s.CC {
			out = append(out, s)
		}
	}
	return out
}

func itoa(i int) string {
	return string(rune('0' + i))
}
