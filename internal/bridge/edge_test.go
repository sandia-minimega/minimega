// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"path/filepath"
	"reflect"
	"testing"
)

func edgeBridge() *Bridge {
	b := testBridge()
	b.taps = map[string]*Tap{
		"t1":    {Name: "t1", VLAN: 101, Namespace: "exp", Owner: "web"},
		"t2":    {Name: "t2", VLAN: 102, Namespace: "exp", Owner: "db"},
		"t3":    {Name: "t3", VLAN: 101, Namespace: "other", Owner: "web"},
		"t4":    {Name: "t4", VLAN: 101, Namespace: "exp", Owner: "web", Defunct: true},
		"host":  {Name: "host", VLAN: 101, Namespace: "exp", Host: true},
		"b1":    {Name: "b1", VLAN: 103, Namespace: "exp", Owner: "fw", Bond: "mega_bond0"},
		"b2":    {Name: "b2", VLAN: 103, Namespace: "exp", Owner: "fw", Bond: "mega_bond0"},
		"anon":  {Name: "anon", VLAN: 101},
		"mtap":  {Name: "mtap", VLAN: 101, Namespace: "exp"},
		"mirro": {Name: "mirro", Namespace: "exp", Host: true},
	}
	b.mirrors["mirro"] = true
	b.trunks["eth1"] = true
	b.tunnels["gre0"] = true

	return b
}

func TestEdgePorts(t *testing.T) {
	b := edgeBridge()

	tests := []struct {
		sel  EdgeSelector
		want []string
	}{
		{EdgeSelector{Namespace: "exp"}, []string{"host", "mega_bond0", "mtap", "t1", "t2"}},
		{EdgeSelector{Namespace: "exp", VLANs: []int{101}}, []string{"host", "mtap", "t1"}},
		{EdgeSelector{Namespace: "exp", VMs: []string{"web", "fw"}}, []string{"mega_bond0", "t1"}},
		{EdgeSelector{Namespace: "other"}, []string{"t3"}},
		{EdgeSelector{Namespace: "none"}, nil},
	}

	for _, tc := range tests {
		if got := b.EdgePorts(tc.sel); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("EdgePorts(%+v) = %v, want %v", tc.sel, got, tc.want)
		}
	}
}

func TestEdgeMirrorHook(t *testing.T) {
	cmds := useFakeCommands(t)
	useFakeSources(t, newFakeSource())

	b := edgeBridge()
	b.tapChan = testTapChan()

	id, err := b.Capture(filepath.Join(t.TempDir(), "edge.pcap"), CaptureConfig{
		Namespace: "exp",
		Ports:     PortsEdge,
		VLANs:     []int{101},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"-- --id=@out get Port mega_tap42 -- --id=@s0 get Port host -- --id=@s1 get Port mtap -- --id=@s2 get Port t1 " +
			"-- --id=@m create Mirror name=mirror-mega_tap42 output_port=@out select_src_port=@s0,@s1,@s2 select_vlan=101",
		`external_ids:minimega-kind="capture-bridge"`,
	} {
		if !cmds.has(want) {
			t.Errorf("missing %q in %q", want, cmds.cmds)
		}
	}

	// a new VM tap of the namespace joins the mirror; one of another does not
	b.taps["t5"] = &Tap{Name: "t5", VLAN: 101}
	b.taps["t6"] = &Tap{Name: "t6", VLAN: 101}
	b.SetTapOwner("t5", "exp", "new")
	b.SetTapOwner("t6", "other", "new")

	if !cmds.has("-- --id=@p get Port t5 -- add Mirror mirror-mega_tap42 select_src_port @p") {
		t.Errorf("hook did not add t5: %q", cmds.cmds)
	}
	if cmds.has("get Port t6") {
		t.Errorf("hook added a tap of another namespace")
	}

	// a tap that moves to another VLAN leaves the mirror
	bridgeLock.Lock()
	b.taps["t1"].VLAN = 102
	b.edgeHook()
	bridgeLock.Unlock()

	if !cmds.has("-- --id=@p get Port t1 -- remove Mirror mirror-mega_tap42 select_src_port @p") {
		t.Errorf("hook did not remove t1: %q", cmds.cmds)
	}

	if err := b.StopCapture(id); err != nil {
		t.Fatal(err)
	}

	if len(b.edgeCaptures) != 0 {
		t.Errorf("edge captures left: %v", b.edgeCaptures)
	}
}
