package types

import (
	"testing"

	"phenix/types/version"

	"gopkg.in/yaml.v3"
)

var experiment = `
apiVersion: v1
kind: Experiment
metadata:
  name: foobar
spec:
  schedules:
  - hostname: suckafish
    clusterNode: compute1
`

var topology = `
apiVersion: phenix.sandia.gov/v1
kind: Topology
metadata:
  name: foobar
spec:
  nodes:
  - type: VirtualMachine
    general:
      hostname: turbine-01
    hardware:
      os_type: linux
      drives:
      - image: bennu.qc2
    network:
      interfaces:
      - name: IF0
        vlan: ot
        address: 192.168.10.1
        mask: 24.
        gateway: 192.168.10.254
        proto: static
        type: ethernet
      - name: mgmt
        vlan: MGMT
        address: 172.16.10.1
        mask: 16.
        proto: static
        type: ethernet
      - name: S0
        vlan: foobar
        address: 172.16.10.1
        mask: 16.
        proto: static
        type: serial
        baud_rate: 9600.
        udp_port: 8989.
        device: "/dev/ttyS0"
`

func TestConfig(t *testing.T) {
	var c Config

	if err := yaml.Unmarshal([]byte(experiment), &c); err != nil {
		t.Log(err)
		t.FailNow()
	}

	t.Logf("%+v", c)

	spec, err := version.GetVersionedSpecForKind(c.Kind, c.APIVersion())
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	t.Logf("%+v", spec)
}
