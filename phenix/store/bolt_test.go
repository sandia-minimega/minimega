package store

import (
	"io/ioutil"
	"os"
	"testing"

	"phenix/types"

	"gopkg.in/yaml.v3"
)

var topology = `
apiVersion: v1
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
        mask: 24
        gateway: 192.168.10.254
        proto: static
        type: ethernet
      - name: mgmt
        vlan: MGMT
        address: 172.16.10.1
        mask: 16
        proto: static
        type: ethernet
`

func TestConfigCreate(t *testing.T) {
	f, err := ioutil.TempFile("/tmp", "phenix")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	defer os.Remove(f.Name())

	b := NewBoltDB()

	if err := b.Init(Path(f.Name())); err != nil {
		t.Log(err)
		t.FailNow()
	}

	var c types.Config

	if err := yaml.Unmarshal([]byte(topology), &c); err != nil {
		t.Log(err)
		t.FailNow()
	}

	if err := b.Create(&c); err != nil {
		t.Log(err)
		t.FailNow()
	}
}

func TestConfigCreateAndGet(t *testing.T) {
	f, err := ioutil.TempFile("/tmp", "phenix")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	defer os.Remove(f.Name())

	b := NewBoltDB()

	if err := b.Init(Path(f.Name())); err != nil {
		t.Log(err)
		t.FailNow()
	}

	var c types.Config

	if err := yaml.Unmarshal([]byte(topology), &c); err != nil {
		t.Log(err)
		t.FailNow()
	}

	if err := b.Create(&c); err != nil {
		t.Log(err)
		t.FailNow()
	}

	c = types.Config{
		Kind: "Topology",
		Metadata: types.ConfigMetadata{
			Name: "foobar",
		},
	}

	if err := b.Get(&c); err != nil {
		t.Log(err)
		t.FailNow()
	}
}

func TestConfigDelete(t *testing.T) {
	f, err := ioutil.TempFile("/tmp", "phenix")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	defer os.Remove(f.Name())

	b := NewBoltDB()

	if err := b.Init(Path(f.Name())); err != nil {
		t.Log(err)
		t.FailNow()
	}

	c, _ := types.NewConfig("topology/foobar")

	if err := b.Delete(c); err != nil {
		t.Log(err)
		t.FailNow()
	}
}
