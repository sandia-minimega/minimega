// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
)

func edgeBridge() *Bridge {
	b := testBridge()
	b.taps = map[string]*Tap{
		"t1":        {Name: "t1", VLAN: 101, Namespace: "exp", Owner: "web"},
		"t2":        {Name: "t2", VLAN: 102, Namespace: "exp", Owner: "db"},
		"t3":        {Name: "t3", VLAN: 101, Namespace: "other", Owner: "web"},
		"t4":        {Name: "t4", VLAN: 101, Namespace: "exp", Owner: "web", Defunct: true},
		"host":      {Name: "host", VLAN: 101, Namespace: "exp", Host: true},
		"b1":        {Name: "b1", VLAN: 103, Namespace: "exp", Owner: "fw", Bond: "mega_bond0"},
		"b2":        {Name: "b2", VLAN: 103, Namespace: "exp", Owner: "fw", Bond: "mega_bond0"},
		"anon":      {Name: "anon", VLAN: 101},
		"mtap":      {Name: "mtap", VLAN: 101, Namespace: "exp"},
		"capmirror": {Name: "capmirror", Namespace: "exp", Host: true},
	}
	b.mirrors["capmirror"] = true
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
		if got := b.edgePorts(tc.sel); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("edgePorts(%+v) = %v, want %v", tc.sel, got, tc.want)
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
		"-- set Interface mega_tap42 type=internal -- --id=@out get Port mega_tap42 -- --id=@s0 get Port host -- --id=@s1 get Port mtap -- --id=@s2 get Port t1 " +
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

// TestDestroyEdgeMirrorFails checks that an edge mirror that Open vSwitch did
// not delete keeps its reconcile loop, so that a later delete can try again.
func TestDestroyEdgeMirrorFails(t *testing.T) {
	useFakeCommands(t)
	useFakeSources(t, newFakeSource())

	b := edgeBridge()
	b.tapChan = testTapChan()

	id, err := b.Capture(filepath.Join(t.TempDir(), "edge.pcap"), CaptureConfig{Namespace: "exp", Ports: PortsEdge})
	if err != nil {
		t.Fatal(err)
	}

	// the reconcile loop runs ovs-vsctl with bridgeLock held
	bridgeLock.Lock()
	fake := ovsExec
	ovsExec = func(args []string) (string, error) {
		return "ovs-vsctl: timeout", errNoSuchPort
	}
	err = b.destroyMirror("mega_tap42")
	_, ok := b.edgeCaptures["mega_tap42"]
	ovsExec = fake
	bridgeLock.Unlock()

	if err == nil || !ok || !b.mirrors["mega_tap42"] {
		t.Errorf("after a failed delete: err %v, edge capture %v, mirror %v", err, ok, b.mirrors["mega_tap42"])
	}

	if err := b.StopCapture(id); err != nil {
		t.Fatal(err)
	}
	if len(b.edgeCaptures) != 0 || b.mirrors["mega_tap42"] {
		t.Errorf("edge mirror left after stop: %v %v", b.edgeCaptures, b.mirrors)
	}
}

// TestEdgeMirrorTapReconnect checks that a tap that leaves the bridge and comes
// back, for example after "vm net connect" on the same bridge, joins the mirror
// again: OVS makes a new port, and the mirror lost the old one.
func TestEdgeMirrorTapReconnect(t *testing.T) {
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
	defer b.StopCapture(id)

	if err := b.RemoveTap("t1"); err != nil {
		t.Fatal(err)
	}
	if err := b.AddTap("t1", "", 101, false); err != nil {
		t.Fatal(err)
	}
	b.SetTapOwner("t1", "exp", "web")

	if !cmds.has("-- --id=@p get Port t1 -- add Mirror mirror-mega_tap42 select_src_port @p") {
		t.Errorf("hook did not add the new port of t1: %q", cmds.cmds)
	}
}

// TestStartEdgeStreamOneTransaction checks that a stream makes its capture
// port and the mirror in one ovs-vsctl transaction, and removes both when the
// start fails after that.
func TestStartEdgeStreamOneTransaction(t *testing.T) {
	cmds := useFakeCommands(t)

	old := openCaptureSource
	t.Cleanup(func() { openCaptureSource = old })
	openCaptureSource = func(string, CaptureConfig) (packetSource, error) {
		return nil, errors.New("no such device")
	}

	b := edgeBridge()
	b.tapChan = testTapChan()

	w, err := pcapfile.NewClassic(&bytes.Buffer{}, 1600)
	if err != nil {
		t.Fatal(err)
	}

	tags := MirrorTags{Namespace: "exp", Kind: MirrorKindNamespace, Capture: "exp/head/1"}
	if _, err := b.StartEdgeStream(EdgeSelector{Namespace: "exp"}, CaptureConfig{}, w, tags); err == nil {
		t.Fatal("StartEdgeStream() did not fail")
	}

	var n int
	for _, v := range cmds.cmds {
		if strings.Contains(v, "add-port test_bridge mega_tap42") {
			n++
			if !strings.Contains(v, "create Mirror name=mirror-mega_tap42") {
				t.Errorf("port made without its mirror: %q", v)
			}
		}
	}
	if n != 1 {
		t.Errorf("port made %d times: %q", n, cmds.cmds)
	}

	for _, want := range []string{
		"get mirror mirror-mega_tap42 -- remove bridge test_bridge mirrors @m",
		"del-port test_bridge mega_tap42",
	} {
		if !cmds.has(want) {
			t.Errorf("missing %q after the failed start in %q", want, cmds.cmds)
		}
	}

	if _, ok := b.taps["mega_tap42"]; ok || b.mirrors["mega_tap42"] || len(b.edgeCaptures) != 0 {
		t.Errorf("capture tap or mirror left: mirrors %v, edge %v", b.mirrors, b.edgeCaptures)
	}
}

// closeWriter is a pcapfile.Writer that records Close.
type closeWriter struct {
	pcapfile.Writer
	closed bool
}

func (w *closeWriter) Close() error {
	w.closed = true
	return w.Writer.Close()
}

// TestStartEdgeStreamFailKeepsWriter checks that a failed start does not close
// the writer: the caller sets the error on the writer, then closes it.
func TestStartEdgeStreamFailKeepsWriter(t *testing.T) {
	for step, want := range map[string]string{"mirror": "add edge mirror failed", "source": "no such device"} {
		t.Run(step, func(t *testing.T) {
			useFakeCommands(t)
			if step == "mirror" {
				ovsExec = func(args []string) (string, error) {
					if strings.Contains(strings.Join(args, " "), "create Mirror") {
						return "", errors.New("mirror limit")
					}
					return "", nil
				}
			}

			old := openCaptureSource
			t.Cleanup(func() { openCaptureSource = old })
			openCaptureSource = func(string, CaptureConfig) (packetSource, error) {
				return nil, errors.New("no such device")
			}

			b := edgeBridge()
			b.tapChan = testTapChan()

			pw, err := pcapfile.NewClassic(&bytes.Buffer{}, 1600)
			if err != nil {
				t.Fatal(err)
			}
			w := &closeWriter{Writer: pw}

			tags := MirrorTags{Namespace: "exp", Kind: MirrorKindNamespace, Capture: "exp/head/1"}
			if _, err := b.StartEdgeStream(EdgeSelector{Namespace: "exp"}, CaptureConfig{}, w, tags); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("StartEdgeStream() error %v, want %q", err, want)
			}
			if w.closed {
				t.Error("StartEdgeStream() closed the writer after a failed start")
			}
		})
	}
}

// TestReconcileLoopEndsOnDestroy checks that the reconcile loop of an edge
// mirror ends when the bridge is destroyed, also when destroy did not delete
// the mirror and so did not close its done channel.
func TestReconcileLoopEndsOnDestroy(t *testing.T) {
	b := testBridge()
	ec := &edgeCapture{ports: map[string]bool{}, done: make(chan struct{})}

	bridgeLock.Lock()
	b.setDestroyed()
	bridgeLock.Unlock()

	ended := make(chan struct{})
	go func() {
		b.reconcileLoop(ec, 5*time.Millisecond)
		close(ended)
	}()

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Error("the reconcile loop of a destroyed bridge did not end")
	}
}
