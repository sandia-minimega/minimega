// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/gonetflow"
)

// TestReadPacketXXXEndsCleanly reads a v5 packet with four full records, as
// minimega writes them, and checks that the stream then ends with io.EOF.
func TestReadPacketXXXEndsCleanly(t *testing.T) {
	b := make([]byte, gonetflow.NETFLOW_HEADER_LEN+4*gonetflow.NETFLOW_RECORD_LEN)
	b[1] = 5 // version

	r := bufio.NewReader(bytes.NewReader(b))

	p, err := readPacketXXX(r)
	if err != nil {
		t.Fatalf("first packet: %v", err)
	}
	if len(p.Records) != 4 {
		t.Errorf("got %d records, want 4", len(p.Records))
	}

	if _, err := readPacketXXX(r); err != io.EOF {
		t.Errorf("after the last packet: err = %v, want io.EOF", err)
	}
}

// TestReadPacketXXXTruncatedHeader checks that a stray byte after the last
// record, the first byte of a truncated next header, is reported as an
// unexpected EOF instead of being discarded along with the padding byte.
func TestReadPacketXXXTruncatedHeader(t *testing.T) {
	for _, tc := range []struct {
		extra int
		want  error
	}{{0, nil}, {1, io.ErrUnexpectedEOF}} {
		b := make([]byte, gonetflow.NETFLOW_HEADER_LEN+4*gonetflow.NETFLOW_RECORD_LEN+tc.extra)
		b[1] = 5
		if _, err := readPacketXXX(bufio.NewReader(bytes.NewReader(b))); err != tc.want {
			t.Errorf("%d extra bytes: err = %v, want %v", tc.extra, err, tc.want)
		}
	}
}

// The files in testdata were written by minimega's raw, gzipped netflow
// capture (capture netflow gzip true; capture netflow bridge mega_bridge
// sandwich.nf.gz) in chapter 12 of the miniclass docs, on two separate runs of
// the miniclass workflow. Each holds one v5 packet of four ICMP flows between
// 10.0.0.10 and 10.0.1.10.
func TestReadPacketXXXDocsCapture(t *testing.T) {
	for _, tc := range []struct {
		file     string
		epochSec uint32
		lasts    [4]uint32
	}{
		{"ch12-run43297321125.nf.gz", 0x6af62230, [4]uint32{0xfcf, 0xfce, 0xfce, 0xfcf}},
		{"ch12-run43276593818.nf.gz", 0x6af60653, [4]uint32{0xfa8, 0xfa8, 0xfa8, 0xfa8}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			zr, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			r := bufio.NewReader(zr)

			p, err := readPacketXXX(r)
			if err != nil {
				t.Fatalf("first packet: %v", err)
			}
			if p.Header.Version != 5 || p.Header.Count != 4 || p.Header.EpochSec != tc.epochSec {
				t.Errorf("header = %+v, want version 5, count 4, epoch %#x", p.Header, tc.epochSec)
			}
			if len(p.Records) != 4 {
				t.Fatalf("got %d records, want 4", len(p.Records))
			}

			// source, destination, input and output interface of each flow
			want := [4][4]string{
				{"10.0.1.10", "10.0.0.10", "42", "40"},
				{"10.0.0.10", "10.0.1.10", "40", "42"},
				{"10.0.0.10", "10.0.1.10", "43", "41"},
				{"10.0.1.10", "10.0.0.10", "41", "43"},
			}
			for i, rec := range p.Records {
				got := [4]string{rec.Src.String(), rec.Dst.String(), itoa(rec.Input), itoa(rec.Output)}
				if got != want[i] {
					t.Errorf("record %d: %v, want %v", i, got, want[i])
				}
				if rec.Protocol != 1 || rec.NumPackets != 5 || rec.NumOctets != 490 {
					t.Errorf("record %d: proto %d, %d packets, %d octets; want ICMP, 5, 490",
						i, rec.Protocol, rec.NumPackets, rec.NumOctets)
				}
				if rec.Last != tc.lasts[i] {
					t.Errorf("record %d: last = %#x, want %#x", i, rec.Last, tc.lasts[i])
				}
			}

			if _, err := readPacketXXX(r); err != io.EOF {
				t.Errorf("after the last packet: err = %v, want io.EOF", err)
			}
		})
	}
}

func itoa(u uint) string { return strconv.FormatUint(uint64(u), 10) }
