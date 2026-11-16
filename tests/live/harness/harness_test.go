// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

// These tests need no deployment: run them with `go test ./tests/live/...`.

func writeResponse(t *testing.T, dir, uuid string, files map[string]string) {
	t.Helper()
	path := filepath.Join(dir, uuid)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(path, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadResponses(t *testing.T) {
	dir := t.TempDir()
	writeResponse(t, dir, "uuid-a", map[string]string{"exitcode": "3", "stdout": "out\n", "stderr": "err\n"})
	writeResponse(t, dir, "uuid-b", map[string]string{"stdout": "sent"})
	writeResponse(t, dir, "uuid-c", map[string]string{"exitcode": "0\n"})

	got, err := readResponses(dir, map[string]string{"uuid-a": "host-a", "uuid-b": "host-b"})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]CCResult{
		"host-a": {Stdout: "out\n", Stderr: "err\n", ExitCode: 3},
		"host-b": {Stdout: "sent", ExitCode: noExitCode},
		"uuid-c": {ExitCode: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d responses %v, want %d", len(got), got, len(want))
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%v: got %+v, want %+v", name, got[name], w)
		}
	}
}

func TestReadResponsesNoDirectory(t *testing.T) {
	got, err := readResponses(filepath.Join(t.TempDir(), "missing"), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want no responses and no error", got, err)
	}
}

func TestReadResponsesBadExitCode(t *testing.T) {
	dir := t.TempDir()
	writeResponse(t, dir, "uuid-a", map[string]string{"exitcode": "not a number"})

	if got, err := readResponses(dir, nil); err == nil {
		t.Fatalf("got %v, want an error for an unparsable exit code", got)
	}
}

func TestMissingNeed(t *testing.T) {
	passed := map[string]bool{"Version": true, "Inventory": true}
	for _, c := range []struct {
		needs []string
		want  string
	}{
		{nil, ""},
		{[]string{"Version", "Inventory"}, ""},
		{[]string{"Inventory", "CCClients"}, "CCClients"},
	} {
		if got := missingNeed(c.needs, passed); got != c.want {
			t.Errorf("missingNeed(%v) = %q, want %q", c.needs, got, c.want)
		}
	}
}

func TestCheckPhases(t *testing.T) {
	noop := func(*testing.T) {}
	ok := []Phase{{"A", noop, nil}, {"B", noop, []string{"A"}}}
	if err := checkPhases(ok); err != nil {
		t.Errorf("valid phases: %v", err)
	}
	for name, bad := range map[string][]Phase{
		"later need":   {{"A", noop, []string{"B"}}, {"B", noop, nil}},
		"unknown need": {{"A", noop, []string{"Nope"}}},
	} {
		if err := checkPhases(bad); err == nil {
			t.Errorf("%v: want an error", name)
		}
	}
}

func writePcap(t *testing.T, pkts ...[]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.pcap")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := pcapgo.NewWriter(f)
	if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		t.Fatal(err)
	}
	for _, p := range pkts {
		ci := gopacket.CaptureInfo{CaptureLength: len(p), Length: len(p)}
		if err := w.WritePacket(ci, p); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func icmpPacket(t *testing.T, dst string, typ uint8) []byte {
	t.Helper()
	buf := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true},
		&layers.Ethernet{SrcMAC: net.HardwareAddr{0, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{0, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4},
		&layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolICMPv4, SrcIP: net.ParseIP("10.10.1.119"), DstIP: net.ParseIP(dst)},
		&layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(typ, 0)},
	)
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCountICMPEcho(t *testing.T) {
	path := writePcap(t,
		icmpPacket(t, "10.10.1.1", layers.ICMPv4TypeEchoRequest),
		icmpPacket(t, "10.10.1.1", layers.ICMPv4TypeEchoRequest),
		icmpPacket(t, "10.10.1.1", layers.ICMPv4TypeEchoReply),
		icmpPacket(t, "10.10.2.1", layers.ICMPv4TypeEchoRequest),
	)
	n, err := CountICMPEcho(path, "10.10.1.1")
	if err != nil || n != 2 {
		t.Fatalf("countICMPEcho = %d, %v; want 2 echo requests to 10.10.1.1", n, err)
	}
}

func TestDistinctColors(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	encode := func() []byte {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	if n, err := DistinctColors(encode()); err != nil || n != 1 {
		t.Errorf("blank image: %d colors, %v; want 1", n, err)
	}
	img.Set(1, 1, color.White)
	if n, err := DistinctColors(encode()); err != nil || n != 2 {
		t.Errorf("one white pixel: %d colors, %v; want 2", n, err)
	}
	if _, err := DistinctColors([]byte("not a png")); err == nil {
		t.Error("garbage: want an error")
	}
}

func TestFBChunks(t *testing.T) {
	write := func(chunks ...string) string {
		path := filepath.Join(t.TempDir(), "x.fb")
		var b bytes.Buffer
		z := gzip.NewWriter(&b)
		for _, c := range chunks {
			fmt.Fprintf(z, "%d %d\r\n%s\r\n", 1000, len(c), c)
		}
		z.Close()
		if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if n, err := FBChunks(write()); err != nil || n != 0 {
		t.Errorf("empty recording: %d chunks, %v; want 0", n, err)
	}
	if n, err := FBChunks(write("abc", "de\r\nf")); err != nil || n != 2 {
		t.Errorf("two chunks: %d, %v; want 2", n, err)
	}
}

func TestExecResult(t *testing.T) {
	res := map[string]CCResult{
		"ok":   {ExitCode: 0},
		"fail": {ExitCode: 1},
		"none": {Stdout: "partial", ExitCode: noExitCode},
		"sig":  {ExitCode: -1}, // killed by a signal: a real result
	}
	for _, vm := range []string{"ok", "fail", "sig"} {
		if r, err := execResult(res, vm); err != nil || r != res[vm] {
			t.Errorf("%v: got %+v, %v", vm, r, err)
		}
	}
	for _, vm := range []string{"none", "missing"} {
		if _, err := execResult(res, vm); err == nil {
			t.Errorf("%v: want an error", vm)
		}
	}
}
