// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/nscapture"
	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"

	"github.com/gopacket/gopacket/pcapgo"
)

func TestParseNSCaptureOptions(t *testing.T) {
	ns := &Namespace{Name: "nscapture-test"}

	o, err := parseNSCaptureOptions(ns, []string{"ports=edge", "vlans=101,102", `filter=tcp port 80`, "snaplen=128",
		"format=pcapng", "attribute=stream", "names=false", "window=500ms", "maxsize=10240M", "bridges=a,b", "vms=web"}, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(o.vlanIDs) != 2 || o.filter != "tcp port 80" || o.snaplen != 128 || o.format != "pcapng" ||
		o.attribute != "stream" || o.names || o.window != 500*time.Millisecond || o.maxSize != 10240<<20 ||
		len(o.bridges) != 2 || o.vms[0] != "web" {
		t.Errorf("options %+v", o)
	}

	if o, err := parseNSCaptureOptions(ns, nil, false); err != nil || o.snaplen != 1600 || o.window != time.Second {
		t.Errorf("defaults %+v %v", o, err)
	}

	for _, opts := range [][]string{
		{"ports=all"},
		{"head=x"},
		{"token=1"},
		{`filter=host "a"`},
		{`filter=a\b`},
		{"filter=tcp port eighty"},
		{"snaplen=13"},
		{"snaplen=65536"},
		{"format=pcapx"},
		{"attribute=port", "format=pcap"},
		{"names=maybe"},
		{"window=50ms"},
		{"window=11s"},
		{"maxsize=1T"},
		{"colour=blue"},
		{"vlans=0"},
	} {
		if _, err := parseNSCaptureOptions(ns, opts, false); err == nil {
			t.Errorf("options %q did not fail", opts)
		}
	}

	if _, err := parseNSCaptureOptions(ns, []string{"head=h"}, true); err == nil {
		t.Error("member options without id and token did not fail")
	}

	o.head, o.id, o.token = "head node", 3, 42
	o.vlanIDs = []int{101}
	args := []string{"capture", "pcap", "namespace", quoteArg("/f/a b.pcap")}
	for _, a := range o.memberArgs() {
		args = append(args, quoteArg(a))
	}

	c, err := minicli.Compile(strings.Join(args, " "))
	if err != nil || c == nil {
		t.Fatalf("member command does not compile: %v", err)
	}

	o2, err := parseNSCaptureOptions(ns, c.ListArgs["option"], true)
	if err != nil {
		t.Fatal(err)
	}
	if o2.head != "head node" || o2.id != 3 || o2.token != 42 || o2.filter != o.filter || c.StringArgs["filename"] != "/f/a b.pcap" {
		t.Errorf("member options %+v from %q", o2, c.ListArgs["option"])
	}
}

func TestNSCapturePatterns(t *testing.T) {
	for in, want := range map[string]string{
		"capture pcap namespace":                     "",
		"capture pcap namespace exp/files/a.pcap":    "exp/files/a.pcap",
		"capture pcap delete namespace all":          "",
		`capture pcap namespace a.pcap filter="tcp"`: "a.pcap",
	} {
		c, err := minicli.Compile(in)
		if err != nil || c == nil {
			t.Fatalf("Compile(%q): %v", in, err)
		}
		if c.StringArgs["filename"] != want {
			t.Errorf("%q: filename %q", in, c.StringArgs["filename"])
		}
	}
}

// TestNSCaptureInProcess runs a capture with this node as head and member:
// member streams send through the local sender, the head merges and stops.
func TestNSCaptureInProcess(t *testing.T) {
	if hostname == "" {
		hostname = "testhost"
	}

	ns := "nscapture-inprocess"
	path := filepath.Join(t.TempDir(), "merged.pcap")

	o := nsCaptureOptions{snaplen: 1600, window: 100 * time.Millisecond, attribute: nscapture.AttributePort,
		head: hostname, id: 1, token: 99}

	hc, err := newHeadCapture(ns, []string{hostname}, o, path, "test")
	if err != nil {
		t.Fatal(err)
	}
	hc.setState(nscapture.StateRunning, "")

	// a member with two bridges, registered as the fan-out would
	m := &memberCapture{
		key:      memberKey{ns: ns, head: hostname, id: 1},
		opts:     o,
		streams:  map[string]*memberStream{},
		headSeen: time.Now(),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	nsMembers.mu.Lock()
	nsMembers.m[m.key] = m
	nsMembers.mu.Unlock()

	base := time.Now().Add(-time.Second)
	for i, br := range []string{"br_a", "br_b"} {
		s := nscapture.NewStream(nscapture.StreamConfig{Namespace: ns, Head: hostname, Host: hostname, Bridge: br,
			ID: 1, Token: 99}, nsSender{}, nil)
		m.streams[br] = &memberStream{bridge: br, stream: s}

		for j := 0; j < 50; j++ {
			ts := base.Add(time.Duration(j*2+i) * time.Millisecond)
			data := make([]byte, 60)
			s.WriteRecord(0, pcapfile.AppendRecord(nil, ts, 60, data), pcapfile.RecordOptions{})
		}
	}

	time.Sleep(60 * time.Millisecond)
	for _, s := range m.streams {
		s.stream.Flush()
	}

	waitForCond(t, "packets", func() bool {
		hc.mu.Lock()
		defer hc.mu.Unlock()
		return hc.sum.Packets == 100
	})

	rows := nsCaptureStopHead(ns, "1", 10*time.Second)
	if len(rows) != 3 || rows[0][3] != "stopped" || rows[0][5] != "100" {
		t.Fatalf("final rows %q", rows)
	}
	for _, row := range rows {
		if len(row) != len(nsCaptureHeader) {
			t.Errorf("row has %d columns", len(row))
		}
	}
	if rows[1][1] != hostname || rows[1][2] != "br_a" || rows[2][2] != "br_b" {
		t.Errorf("stream rows %q", rows[1:])
	}

	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Error("member did not stop on OpStop")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	r, err := pcapgo.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	var last int64
	var n int
	for {
		_, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if ts := ci.Timestamp.UnixNano(); ts < last {
			t.Errorf("record %d out of order", n)
		} else {
			last = ts
		}
		n++
	}
	if n != 100 {
		t.Errorf("file has %d records, want 100", n)
	}

	if len(nsHeads.list(ns)) != 0 {
		t.Error("capture still listed after delete")
	}
}

func waitForCond(t *testing.T, what string, fn func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %v", what)
}
