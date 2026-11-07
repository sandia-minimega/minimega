// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package pcapfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net/netip"
	"testing"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
)

// ngBlock is one block that the test walker found.
type ngBlock struct {
	typ  uint32
	body []byte
}

// walkBlocks splits b into blocks. It fails when a block is not whole.
func walkBlocks(t *testing.T, b []byte) []ngBlock {
	t.Helper()

	var blocks []ngBlock
	for len(b) > 0 {
		if len(b) < 12 {
			t.Fatalf("partial block: %d bytes", len(b))
		}

		typ := binary.LittleEndian.Uint32(b[0:])
		n := int(binary.LittleEndian.Uint32(b[4:]))
		if n%4 != 0 || n < 12 || n > len(b) {
			t.Fatalf("bad block length %d, %d bytes left", n, len(b))
		}
		if tail := int(binary.LittleEndian.Uint32(b[n-4:])); tail != n {
			t.Fatalf("leading length %d, trailing length %d", n, tail)
		}

		blocks = append(blocks, ngBlock{typ: typ, body: b[8 : n-4]})
		b = b[n:]
	}

	return blocks
}

// parseOptions decodes the options in b into a map of code to values.
func parseOptions(t *testing.T, b []byte) map[uint16][][]byte {
	t.Helper()

	opts := map[uint16][][]byte{}
	for len(b) >= 4 {
		code := binary.LittleEndian.Uint16(b[0:])
		n := int(binary.LittleEndian.Uint16(b[2:]))
		if code == ngOptEnd {
			return opts
		}
		if 4+n+pad4(n) > len(b) {
			t.Fatalf("option %d with length %d does not fit", code, n)
		}

		opts[code] = append(opts[code], b[4:4+n])
		b = b[4+n+pad4(n):]
	}

	if len(b) != 0 {
		t.Fatalf("%d bytes left after options", len(b))
	}

	return opts
}

func timeOf(b []byte) time.Time {
	ns := uint64(binary.LittleEndian.Uint32(b[0:]))<<32 | uint64(binary.LittleEndian.Uint32(b[4:]))
	return time.Unix(0, int64(ns))
}

func TestNgRoundTrip(t *testing.T) {
	var out chunks

	w, err := NewNg(&out, Section{UserAppl: "minimega", OS: "linux", Hardware: "x86_64", Comment: "test"})
	if err != nil {
		t.Fatal(err)
	}

	if len(out.writes) != 1 || out.Len() == 0 {
		t.Fatalf("section header not written at once: %d writes", len(out.writes))
	}

	if w.Format() != FormatPcapng {
		t.Errorf("Format() = %q", w.Format())
	}

	id0, err := w.AddInterface(Interface{Name: "mega_tap0", Description: "vm foo", Filter: "tcp", SnapLen: 1600, Comment: "first"})
	if err != nil || id0 != 0 {
		t.Fatalf("AddInterface = %d, %v", id0, err)
	}

	if err := w.WriteNames([]Name{
		{Addr: "10.0.0.1", Names: []string{"foo", "foo.local"}},
		{Addr: "not an address", Names: []string{"bad"}},
		{Addr: "fd00::1", Names: []string{"bar"}},
	}); err != nil {
		t.Fatal(err)
	}

	base := time.Unix(1762351353, 123456789)
	var frames [][]byte

	write := func(iface uint32, i int, opt RecordOptions) {
		data := bytes.Repeat([]byte{byte(i)}, 60+i%1000)
		frames = append(frames, data)

		rec := AppendRecord(nil, base.Add(time.Duration(i)), len(data)+4, data)
		if err := w.WriteRecord(iface, rec, opt); err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 2000; i++ {
		var opt RecordOptions
		switch i % 4 {
		case 1:
			opt = RecordOptions{Inbound: true, DropsBefore: uint64(i), Comment: "in"}
		case 2:
			opt = RecordOptions{Outbound: true}
		}
		write(0, i, opt)
	}

	id1, err := w.AddInterface(Interface{Name: "mega_tap1", SnapLen: 0})
	if err != nil || id1 != 1 {
		t.Fatalf("AddInterface = %d, %v", id1, err)
	}

	for i := 2000; i < 3000; i++ {
		write(uint32(i%2), i, RecordOptions{})
	}

	if err := w.WriteStats(0, Stats{Start: base, End: base.Add(time.Second), Received: 10, IfDrop: 1, OSDrop: 2, Delivered: 7}, "done"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteStats(1, Stats{Received: 5}, ""); err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if got, want := w.Size(), int64(out.Len()); got != want {
		t.Errorf("Size() = %d, file has %d bytes", got, want)
	}

	if len(out.writes) < 3 {
		t.Errorf("only %d writes, want the buffer to flush more than once", len(out.writes))
	}
	for _, chunk := range out.writes {
		walkBlocks(t, chunk)
	}

	r, err := pcapgo.NewNgReader(bytes.NewReader(out.Bytes()), pcapgo.DefaultNgReaderOptions)
	if err != nil {
		t.Fatal(err)
	}

	si := r.SectionInfo()
	if si.Application != "minimega" || si.OS != "linux" || si.Hardware != "x86_64" || si.Comment != "test" {
		t.Errorf("section info %+v", si)
	}

	for i := 0; i < len(frames); i++ {
		data, ci, err := r.ReadPacketData()
		if err != nil {
			t.Fatalf("packet %d: %v", i, err)
		}

		want := 0
		if i >= 2000 {
			want = i % 2
		}

		if ci.InterfaceIndex != want {
			t.Errorf("packet %d: interface %d, want %d", i, ci.InterfaceIndex, want)
		}
		if !ci.Timestamp.Equal(base.Add(time.Duration(i))) {
			t.Errorf("packet %d: time %v, want %v", i, ci.Timestamp, base.Add(time.Duration(i)))
		}
		if ci.CaptureLength != len(frames[i]) || ci.Length != len(frames[i])+4 {
			t.Errorf("packet %d: lengths %d/%d", i, ci.CaptureLength, ci.Length)
		}
		if !bytes.Equal(data, frames[i]) {
			t.Errorf("packet %d: data differs", i)
		}
	}

	if _, _, err := r.ReadPacketData(); err != io.EOF {
		t.Errorf("after last packet: %v, want EOF", err)
	}

	if r.NInterfaces() != 2 {
		t.Fatalf("%d interfaces, want 2", r.NInterfaces())
	}

	i0, _ := r.Interface(0)
	if i0.Name != "mega_tap0" || i0.Description != "vm foo" || i0.Filter != "tcp" || i0.SnapLength != 1600 || i0.Comment != "first" || i0.TimestampResolution != 9 {
		t.Errorf("interface 0: %+v", i0)
	}
	if s := i0.Statistics; s.PacketsReceived != 10 || s.PacketsDropped != 1 || s.Comment != "done" || !s.StartTime.Equal(base) {
		t.Errorf("interface 0 statistics: %+v", s)
	}

	i1, _ := r.Interface(1)
	if i1.Name != "mega_tap1" || i1.SnapLength != 0 || i1.TimestampResolution != 9 || i1.Statistics.PacketsReceived != 5 {
		t.Errorf("interface 1: %+v", i1)
	}

	if r.NNames() != 2 {
		t.Fatalf("%d names, want 2", r.NNames())
	}

	n0, _ := r.Name(0)
	if a, ok := n0.Addr.(*pcapgo.NgIPAddress); !ok || a.Addr != netip.MustParseAddr("10.0.0.1") || len(n0.Names) != 2 || n0.Names[1] != "foo.local" {
		t.Errorf("name 0: %+v %v", n0.Addr, n0.Names)
	}

	n1, _ := r.Name(1)
	if a, ok := n1.Addr.(*pcapgo.NgIPAddress); !ok || a.Addr != netip.MustParseAddr("fd00::1") || len(n1.Names) != 1 || n1.Names[0] != "bar" {
		t.Errorf("name 1: %+v %v", n1.Addr, n1.Names)
	}
}

func TestNgPacketOptions(t *testing.T) {
	var out bytes.Buffer

	w, err := NewNg(&out, Section{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.AddInterface(Interface{Name: "eth0"}); err != nil {
		t.Fatal(err)
	}

	ts := time.Unix(1762351353, 1)
	opts := []RecordOptions{
		{},
		{Inbound: true},
		{Outbound: true, DropsBefore: 42, Comment: "hello"},
	}
	for _, opt := range opts {
		if err := w.WriteRecord(0, AppendRecord(nil, ts, 3, []byte{1, 2, 3}), opt); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r, err := pcapgo.NewNgReader(bytes.NewReader(out.Bytes()), pcapgo.DefaultNgReaderOptions)
	if err != nil {
		t.Fatal(err)
	}

	for i, opt := range opts {
		data, ci, po, err := r.ReadPacketDataWithOptions()
		if err != nil {
			t.Fatalf("packet %d: %v", i, err)
		}
		if !bytes.Equal(data, []byte{1, 2, 3}) || !ci.Timestamp.Equal(ts) {
			t.Errorf("packet %d: data %v, time %v", i, data, ci.Timestamp)
		}

		var dir pcapgo.NgEpbFlag
		if po.Flags != nil {
			dir = po.Flags.Direction
		}

		switch {
		case opt.Inbound && dir != pcapgo.NgEpbFlagDirectionInbound,
			opt.Outbound && dir != pcapgo.NgEpbFlagDirectionOutbound,
			!opt.Inbound && !opt.Outbound && po.Flags != nil:
			t.Errorf("packet %d: flags %+v", i, po.Flags)
		}

		if (opt.DropsBefore == 0) != (po.DropCount == nil) || (po.DropCount != nil && *po.DropCount != opt.DropsBefore) {
			t.Errorf("packet %d: drop count %v", i, po.DropCount)
		}

		if opt.Comment != "" && (len(po.Comments) != 1 || po.Comments[0] != opt.Comment) {
			t.Errorf("packet %d: comments %v", i, po.Comments)
		}
	}
}

func TestNgOptionValues(t *testing.T) {
	var out bytes.Buffer

	w, err := NewNg(&out, Section{OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.AddInterface(Interface{Name: "a", Filter: "udp", SnapLen: 99}); err != nil {
		t.Fatal(err)
	}

	start := time.Unix(1762351353, 5)
	end := time.Unix(1762351400, 987654321)
	ts := time.Unix(1762351360, 7)

	if err := w.WriteRecord(0, AppendRecord(nil, ts, 10, []byte{9, 9, 9, 9, 9}), RecordOptions{Outbound: true, DropsBefore: 3, Comment: "c"}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteStats(0, Stats{Start: start, End: end, Received: 1, IfDrop: 2, OSDrop: 3, Delivered: 4}, "s"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteNames([]Name{{Addr: "192.168.1.2", Names: []string{"x", "yz"}}, {Addr: "::1", Names: []string{"lo"}}}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteNames(nil); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteNames([]Name{{Addr: "bad", Names: []string{"x"}}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	blocks := walkBlocks(t, out.Bytes())
	var types []uint32
	for _, b := range blocks {
		types = append(types, b.typ)
	}
	want := []uint32{ngBlockSHB, ngBlockIDB, ngBlockEPB, ngBlockISB, ngBlockNRB}
	if len(types) != len(want) {
		t.Fatalf("block types %x, want %x", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("block types %x, want %x", types, want)
		}
	}

	shb := blocks[0].body
	if binary.LittleEndian.Uint32(shb[0:]) != ngByteOrderMagic || binary.LittleEndian.Uint16(shb[4:]) != 1 ||
		binary.LittleEndian.Uint16(shb[6:]) != 0 || binary.LittleEndian.Uint64(shb[8:]) != ^uint64(0) {
		t.Errorf("section header %x", shb[:16])
	}
	if o := parseOptions(t, shb[16:]); string(o[ngOptSHBOS][0]) != "linux" || len(o) != 1 {
		t.Errorf("section options %q", o)
	}

	idb := blocks[1].body
	if binary.LittleEndian.Uint16(idb[0:]) != LinkTypeEthernet || binary.LittleEndian.Uint32(idb[4:]) != 99 {
		t.Errorf("interface header %x", idb[:8])
	}
	o := parseOptions(t, idb[8:])
	if !bytes.Equal(o[ngOptIfTsresol][0], []byte{9}) || !bytes.Equal(o[ngOptIfFilter][0], []byte("\x00udp")) || string(o[ngOptIfName][0]) != "a" {
		t.Errorf("interface options %q", o)
	}

	epb := blocks[2].body
	if binary.LittleEndian.Uint32(epb[0:]) != 0 || !timeOf(epb[4:]).Equal(ts) ||
		binary.LittleEndian.Uint32(epb[12:]) != 5 || binary.LittleEndian.Uint32(epb[16:]) != 10 {
		t.Errorf("packet header %x", epb[:20])
	}
	if !bytes.Equal(epb[20:28], []byte{9, 9, 9, 9, 9, 0, 0, 0}) {
		t.Errorf("packet data %x", epb[20:28])
	}
	o = parseOptions(t, epb[28:])
	if binary.LittleEndian.Uint32(o[ngOptEPBFlags][0]) != 2 || binary.LittleEndian.Uint64(o[ngOptEPBDropCount][0]) != 3 || string(o[ngOptComment][0]) != "c" {
		t.Errorf("packet options %q", o)
	}

	isb := blocks[3].body
	if binary.LittleEndian.Uint32(isb[0:]) != 0 || !timeOf(isb[4:]).Equal(end) {
		t.Errorf("statistics header %x", isb[:12])
	}
	o = parseOptions(t, isb[12:])
	if !timeOf(o[ngOptISBStartTime][0]).Equal(start) || !timeOf(o[ngOptISBEndTime][0]).Equal(end) {
		t.Errorf("statistics times %x %x", o[ngOptISBStartTime], o[ngOptISBEndTime])
	}
	for code, v := range map[uint16]uint64{ngOptISBIfRecv: 1, ngOptISBIfDrop: 2, ngOptISBOSDrop: 3, ngOptISBUsrDeliv: 4} {
		if got := binary.LittleEndian.Uint64(o[code][0]); got != v {
			t.Errorf("statistics option %d = %d, want %d", code, got, v)
		}
	}
	if string(o[ngOptComment][0]) != "s" {
		t.Errorf("statistics comment %q", o[ngOptComment])
	}

	nrb := parseOptions(t, blocks[4].body)
	if !bytes.Equal(nrb[ngRecordIPv4][0], []byte("\xc0\xa8\x01\x02x\x00yz\x00")) {
		t.Errorf("ipv4 record %q", nrb[ngRecordIPv4])
	}
	lo := netip.IPv6Loopback().As16()
	if !bytes.Equal(nrb[ngRecordIPv6][0], append(lo[:], "lo\x00"...)) {
		t.Errorf("ipv6 record %q", nrb[ngRecordIPv6])
	}
	if b := blocks[4].body; !bytes.Equal(b[len(b)-4:], []byte{0, 0, 0, 0}) {
		t.Errorf("name block does not end with nrb_record_end: %x", b)
	}
}

func TestNgErrors(t *testing.T) {
	var out bytes.Buffer

	w, err := NewNg(&out, Section{})
	if err != nil {
		t.Fatal(err)
	}

	rec := AppendRecord(nil, time.Unix(1, 0), 4, []byte{1, 2, 3, 4})

	if err := w.WriteRecord(0, rec, RecordOptions{}); !errors.Is(err, errUnknownInterface) {
		t.Errorf("record before interface: %v", err)
	}
	if err := w.WriteStats(0, Stats{}, ""); !errors.Is(err, errUnknownInterface) {
		t.Errorf("stats before interface: %v", err)
	}

	if _, err := w.AddInterface(Interface{}); err != nil {
		t.Fatal(err)
	}

	if err := w.WriteRecord(1, rec, RecordOptions{}); !errors.Is(err, errUnknownInterface) {
		t.Errorf("unknown interface: %v", err)
	}
	if err := w.WriteRecord(0, rec[:10], RecordOptions{}); !errors.Is(err, ErrBadRecord) {
		t.Errorf("short record: %v", err)
	}
	if err := w.WriteRecord(0, rec[:len(rec)-1], RecordOptions{}); !errors.Is(err, ErrBadRecord) {
		t.Errorf("truncated record: %v", err)
	}
	if err := w.WriteRecord(0, rec, RecordOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	size := w.Size()

	if err := w.WriteRecord(0, rec, RecordOptions{}); !errors.Is(err, ErrClosed) {
		t.Errorf("record after Close: %v", err)
	}
	if _, err := w.AddInterface(Interface{}); !errors.Is(err, ErrClosed) {
		t.Errorf("interface after Close: %v", err)
	}
	if err := w.WriteStats(0, Stats{}, ""); !errors.Is(err, ErrClosed) {
		t.Errorf("stats after Close: %v", err)
	}
	if err := w.WriteNames([]Name{{Addr: "10.0.0.1", Names: []string{"a"}}}); !errors.Is(err, ErrClosed) {
		t.Errorf("names after Close: %v", err)
	}
	if w.Size() != size || int64(out.Len()) != size {
		t.Errorf("size changed after Close: %d, file %d, want %d", w.Size(), out.Len(), size)
	}

	walkBlocks(t, out.Bytes())
}

type failWriter struct{ n int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.n == 0 {
		return 0, errors.New("disk full")
	}
	f.n--
	return len(p), nil
}

func TestNgWriteError(t *testing.T) {
	if _, err := NewNg(&failWriter{}, Section{}); err == nil {
		t.Error("NewNg did not return the write error")
	}

	w, err := NewNg(&failWriter{n: 1}, Section{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.AddInterface(Interface{}); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err == nil {
		t.Error("Flush did not return the write error")
	}
	if err := w.WriteRecord(0, AppendRecord(nil, time.Unix(1, 0), 1, []byte{1}), RecordOptions{}); err == nil {
		t.Error("WriteRecord after a write error did not fail")
	}
}
