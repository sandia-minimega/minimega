package main

import (
	"errors"
	"strconv"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

type stopTestCapture struct {
	err   error
	stops *int
}

func (c stopTestCapture) Type() string {
	return "pcap"
}

func (c stopTestCapture) Row(int) []string {
	return nil
}

func (c stopTestCapture) Stop() error {
	(*c.stops)++

	return c.err
}

func TestCapturesStopContinuesAfterError(t *testing.T) {
	stopErr := errors.New("stop failed")
	var stops int
	captures := captures{
		m: map[int]capture{
			0: stopTestCapture{err: stopErr, stops: &stops},
			1: stopTestCapture{stops: &stops},
		},
	}

	err := captures.stop(func(capture) bool {
		return true
	})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected joined stop error, got %v", err)
	}
	if stops != 2 {
		t.Fatalf("expected both captures to be stopped, got %d stops", stops)
	}
	if len(captures.m) != 0 {
		t.Fatalf("expected all stopped captures to be removed, got %d entries", len(captures.m))
	}
}

func captureTestVM(name string) VM {
	return &KvmVM{BaseVM: &BaseVM{Name: name}}
}

func noBridges(t *testing.T) {
	t.Helper()

	old := lookupBridge
	t.Cleanup(func() { lookupBridge = old })

	lookupBridge = func(string) (*bridge.Bridge, bool) { return nil, false }
}

func newTestCaptures() *captures {
	return &captures{m: map[int]capture{}, counter: NewCounter()}
}

func TestCaptureStopErrors(t *testing.T) {
	noBridges(t)

	c := newTestCaptures()
	c.addCapture(&pcapVMCapture{pcapCapture: pcapCapture{Bridge: "br"}, VM: captureTestVM("foo"), Interface: 0})
	c.addCapture(&pcapBridgeCapture{pcapCapture: pcapCapture{Bridge: "br"}})
	c.addCapture(&netflowCapture{Bridge: "br", Path: "x"})

	exists := func(s string) bool { return s == "foo" || s == "bar" }
	one := 1

	if err := c.StopVM("bar", nil, exists); err == nil || err.Error() != "no capture for VM bar" {
		t.Errorf("StopVM(bar) = %v", err)
	}
	if err := c.StopVM("foo", &one, exists); err == nil || err.Error() != "no capture on interface 1 of VM foo" {
		t.Errorf("StopVM(foo, 1) = %v", err)
	}
	if err := c.StopVM("baz", nil, exists); err == nil || !isVMNotFound(err.Error()) {
		t.Errorf("StopVM(baz) = %v", err)
	}
	if err := c.StopBridge("other", "pcap"); err == nil || err.Error() != "no pcap capture on bridge other" {
		t.Errorf("StopBridge(other) = %v", err)
	}
	if err := c.StopID(2); err == nil {
		t.Error("StopID on a netflow capture did not fail")
	}
	if err := c.StopID(9); err == nil {
		t.Error("StopID on an unknown ID did not fail")
	}

	// without bridges, stops succeed and do not create a bridge
	if err := c.StopVM("foo", nil, exists); err != nil {
		t.Errorf("StopVM(foo) = %v", err)
	}
	if err := c.StopID(1); err != nil {
		t.Errorf("StopID(1) = %v", err)
	}
	if len(c.m) != 1 {
		t.Errorf("captures left: %v", c.m)
	}
}

func TestCaptureStopType(t *testing.T) {
	noBridges(t)

	c := newTestCaptures()
	c.addCapture(&pcapBridgeCapture{pcapCapture: pcapCapture{Bridge: "br"}})
	c.addCapture(&netflowCapture{Bridge: "br", Path: "x"})

	if err := c.StopType("pcap"); err != nil {
		t.Fatal(err)
	}

	if len(c.m) != 1 || c.m[1].Type() != "netflow" {
		t.Errorf("clear capture pcap left %v", c.m)
	}
}

func TestCaptureRows(t *testing.T) {
	noBridges(t)

	c := newTestCaptures()
	for i := 0; i < 20; i++ {
		c.addCapture(&pcapBridgeCapture{pcapCapture: pcapCapture{Bridge: "br", Path: "/f"}})
	}
	c.addCapture(&netflowCapture{Bridge: "br", Path: "tcp:host:1"})

	rows := c.Rows()
	if len(rows) != 21 {
		t.Fatalf("got %d rows", len(rows))
	}

	for i, row := range rows {
		if len(row) != len(captureHeader) {
			t.Fatalf("row %d has %d columns", i, len(row))
		}
		if row[5] != strconv.Itoa(i) {
			t.Errorf("row %d has ID %v", i, row[5])
		}
	}

	if captureHeader[len(captureHeader)-1] != "path" || rows[0][len(rows[0])-1] != "/f" {
		t.Error("path is not the last column")
	}

	if rows[0][6] != "failed" || rows[0][15] != "bridge br is gone" {
		t.Errorf("capture without bridge: %v", rows[0])
	}

	if rows[20][6] != "running" || rows[20][1] != "netflow" {
		t.Errorf("netflow row: %v", rows[20])
	}
}

func TestParseCaptureOptions(t *testing.T) {
	base := bridge.CaptureConfig{Filter: "udp", SnapLen: 200}

	c, err := parseCaptureOptions(captureKindVM, base, []string{"snaplen=128", "filter=tcp port 80", "maxsize=10240M", "direction=in"})
	if err != nil {
		t.Fatal(err)
	}

	want := bridge.CaptureConfig{Filter: "tcp port 80", SnapLen: 128, MaxSize: 10240 << 20, Direction: "in"}
	if c != want {
		t.Errorf("got %+v, want %+v", c, want)
	}

	if c, err := parseCaptureOptions(captureKindBridge, base, nil); err != nil || c != base {
		t.Errorf("no options: %+v %v", c, err)
	}

	if c, err := parseCaptureOptions(captureKindVM, base, []string{"filter="}); err != nil || c.Filter != "" {
		t.Errorf("empty filter: %+v %v", c, err)
	}

	for _, opts := range [][]string{
		{"snaplen=13"},
		{"snaplen=262145"},
		{"snaplen=x"},
		{"filter=tcp port eighty"},
		{"maxsize=1T"},
		{"direction=up"},
		{"colour=blue"},
		{"novalue"},
		{"snaplen=100", "snaplen=200"},
	} {
		if _, err := parseCaptureOptions(captureKindVM, base, opts); err == nil {
			t.Errorf("options %q did not fail", opts)
		}
	}

	if _, err := parseCaptureOptions(captureKindBridge, base, []string{"direction=in"}); err == nil {
		t.Error("direction on a bridge capture did not fail")
	}

	if err := checkCaptureFilename("snaplen=100"); err != errFilenameOption {
		t.Errorf("file name with = : %v", err)
	}
}

func TestCapturePatterns(t *testing.T) {
	tests := []struct {
		in   string
		args map[string]string
		opts []string
	}{
		{"capture pcap bridge foo foo.pcap", map[string]string{"bridge": "foo", "filename": "foo.pcap"}, nil},
		{`capture pcap vm web 0 web.pcap filter="tcp port 80" snaplen=128`,
			map[string]string{"vm": "web", "interface": "0", "filename": "web.pcap"},
			[]string{"filter=tcp port 80", "snaplen=128"}},
		{"capture pcap delete id 3", map[string]string{"id": "3"}, nil},
		{`capture pcap filter ""`, map[string]string{"bpf": ""}, nil},
	}

	for _, tc := range tests {
		c, err := minicli.Compile(tc.in)
		if err != nil || c == nil {
			t.Fatalf("Compile(%q): %v", tc.in, err)
		}

		for k, v := range tc.args {
			if got, ok := c.StringArgs[k]; !ok || got != v {
				t.Errorf("%q: arg %v = %q, want %q", tc.in, k, got, v)
			}
		}

		if len(tc.opts) != len(c.ListArgs["option"]) {
			t.Errorf("%q: options %q, want %q", tc.in, c.ListArgs["option"], tc.opts)
		}
		for i := range tc.opts {
			if c.ListArgs["option"][i] != tc.opts[i] {
				t.Errorf("%q: options %q, want %q", tc.in, c.ListArgs["option"], tc.opts)
			}
		}

		// the command written again for another host compiles the same
		c2, err := minicli.Compile(c.Original)
		if err != nil || c2 == nil || len(c2.ListArgs["option"]) != len(tc.opts) {
			t.Errorf("Compile(%q) after round trip: %v", c.Original, err)
		}
	}
}
