// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"github.com/gopacket/gopacket/pcapgo"
)

// TestOVSCapture runs captures on a real Open vSwitch bridge. It needs root
// and a running Open vSwitch, so it runs only with MINIMEGA_OVS_TEST=1.
func TestOVSCapture(t *testing.T) {
	if os.Getenv("MINIMEGA_OVS_TEST") != "1" {
		t.Skip("set MINIMEGA_OVS_TEST=1 to run with Open vSwitch")
	}

	bs := NewBridges("mmtest_br", "mmtest_tap%v", "mmtest_bond%v")
	t.Cleanup(func() { bs.Destroy() })

	br, err := bs.Get("mmtest_br")
	if err != nil {
		t.Fatal(err)
	}

	tap1, err := br.CreateHostTap("", 101)
	if err != nil {
		t.Fatal(err)
	}
	tap2, err := br.CreateHostTap("", 101)
	if err != nil {
		t.Fatal(err)
	}
	br.SetTapOwner(tap1, "exp", "")
	br.SetTapOwner(tap2, "other", "")

	// send frames from the host out of tap; the bridge receives them on tap
	send := func(tap string, n int, v byte) {
		h, err := pcap.OpenLive(tap, 1600, false, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()

		f := make([]byte, 60)
		copy(f, layers.EthernetBroadcast)
		copy(f[6:], net.HardwareAddr{2, 0, 0, 0, 0, v})
		f[12], f[13] = 0x88, 0xb5 // local experimental ethertype
		for i := 0; i < n; i++ {
			if err := h.WritePacketData(f); err != nil {
				t.Fatal(err)
			}
		}
	}

	count := func(path string) (n int, tagged int) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := pcapgo.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for {
			d, _, err := r.ReadPacketData()
			if errors.Is(err, io.EOF) {
				return
			} else if err != nil {
				t.Fatal(err)
			}
			if d[6] == 2 && d[7] == 0 {
				n++
				if d[12] == 0x81 && d[13] == 0x00 {
					tagged++
				}
			}
		}
	}

	dir := t.TempDir()

	// edge capture of namespace exp: frames from tap1 only, tagged with VLAN 101
	edge := filepath.Join(dir, "edge.pcap")
	id, err := br.Capture(edge, CaptureConfig{Namespace: "exp", Ports: PortsEdge, VLANs: []int{101}})
	if err != nil {
		t.Fatal(err)
	}

	mirrors, err := ListMirrors("mmtest_br")
	if err != nil {
		t.Fatal(err)
	}
	if len(mirrors) != 1 || mirrors[0].Kind != MirrorKindBridge || mirrors[0].Namespace != "exp" {
		t.Errorf("mirrors %+v", mirrors)
	}

	time.Sleep(500 * time.Millisecond)
	send(tap1, 10, 1)
	send(tap2, 5, 2)
	time.Sleep(500 * time.Millisecond)

	// Open vSwitch updates mirror statistics every 5 s by default
	var applied bool
	for deadline := time.Now().Add(12 * time.Second); !applied && time.Now().Before(deadline); {
		time.Sleep(500 * time.Millisecond)
		applied, err = br.MirrorApplied(id)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !applied {
		t.Error("Open vSwitch did not apply the edge mirror")
	}

	if err := br.StopCapture(id); err != nil {
		t.Fatal(err)
	}

	if n, tagged := count(edge); n != 10 || tagged != 10 {
		t.Errorf("edge capture has %d frames (%d tagged), want 10", n, tagged)
	}

	// an all-ports capture sees both taps and leaves no tap or mirror
	all := filepath.Join(dir, "all.pcapng")
	id, err = br.Capture(all, CaptureConfig{Format: "pcapng"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	send(tap1, 3, 1)
	send(tap2, 4, 2)
	time.Sleep(500 * time.Millisecond)
	if err := br.StopCapture(id); err != nil {
		t.Fatal(err)
	}

	out, err := ovsCmdWrapper([]string{"list-ports", "mmtest_br"})
	if err != nil {
		t.Fatal(err)
	}
	if ports := strings.Fields(out); len(ports) != 2 {
		t.Errorf("ports after stop: %v, want only the two host taps", ports)
	}
	if mirrors, _ := ListMirrors("mmtest_br"); len(mirrors) != 0 {
		t.Errorf("mirrors after stop: %+v", mirrors)
	}

	f, err := os.Open(all)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for {
		d, _, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if d[6] == 2 && d[7] == 0 {
			n++
		}
	}
	if n != 7 {
		t.Errorf("all-ports capture has %d frames, want 7", n)
	}

	// an edge stream writes to any pcapfile.Writer and tags its mirror
	var buf bytes.Buffer
	w, err := pcapfile.NewClassic(&buf, 1600)
	if err != nil {
		t.Fatal(err)
	}
	sel := EdgeSelector{Namespace: "exp"}
	id, err = br.StartEdgeStream(sel, CaptureConfig{SnapLen: 1600}, w, MirrorTags{Namespace: "exp", Kind: MirrorKindNamespace, Capture: "exp/head/1"})
	if err != nil {
		t.Fatal(err)
	}
	if mirrors, _ := ListMirrors("mmtest_br"); len(mirrors) != 1 || mirrors[0].Kind != MirrorKindNamespace {
		t.Errorf("stream mirrors %+v", mirrors)
	}
	time.Sleep(500 * time.Millisecond)
	send(tap1, 6, 1)
	send(tap2, 2, 2)
	time.Sleep(500 * time.Millisecond)
	if err := br.StopCapture(id); err != nil {
		t.Fatal(err)
	}
	r2, err := pcapgo.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	n = 0
	for {
		d, _, err := r2.ReadPacketData()
		if err != nil {
			break
		}
		if d[6] == 2 && d[7] == 0 {
			n++
		}
	}
	if n != 6 {
		t.Errorf("edge stream has %d frames, want 6", n)
	}

	if _, err := br.StartEdgeStream(EdgeSelector{Namespace: "none"}, CaptureConfig{}, w, MirrorTags{}); !errors.Is(err, ErrNoEdgePorts) {
		t.Errorf("stream without edge ports: %v", err)
	}

	// a mirror and port of a dead minimega are removed
	if _, err := ovsCmdWrapper([]string{"set", "Port", tap2, "external_ids:minimega-pid=999999999"}); err != nil {
		t.Fatal(err)
	}
	if err := CleanStaleMirrors(); err != nil {
		t.Fatal(err)
	}
	out, _ = ovsCmdWrapper([]string{"list-ports", "mmtest_br"})
	if strings.Contains(out, tap2) {
		t.Errorf("stale port %v not removed: %v", tap2, out)
	}
}
