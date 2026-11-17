// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package pcapfile

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestSizes checks each size function against the bytes that a writer adds.
func TestSizes(t *testing.T) {
	now := time.Unix(1700000000, 5)

	for _, format := range []string{"pcap", FormatPcapng} {
		var buf bytes.Buffer

		var w Writer
		var err error
		if format == FormatPcapng {
			w, err = NewNg(&buf, Section{UserAppl: "test"})
		} else {
			w, err = NewClassic(&buf, 1600)
		}
		if err != nil {
			t.Fatal(err)
		}

		check := func(what string, want int, write func() error) {
			t.Helper()

			before := w.Size()
			if err := write(); err != nil {
				t.Fatalf("%v %v: %v", format, what, err)
			}
			if got := int(w.Size() - before); got != want {
				t.Errorf("%v %v: writer added %d bytes, size function = %d", format, what, got, want)
			}
		}

		for _, i := range []Interface{
			{},
			{Name: "a/b/c", Description: "edge", Filter: "tcp port 80", Comment: "odd", SnapLen: 1600},
		} {
			check("interface "+i.Name, InterfaceSize(format, i), func() error {
				_, err := w.AddInterface(i)
				return err
			})
		}

		for _, n := range []int{0, 1, 3, 60, 61, 1514} {
			for _, opt := range []RecordOptions{
				{},
				{Inbound: true},
				{Outbound: true, DropsBefore: 9},
				{Inbound: true, DropsBefore: 1, Comment: "late: after the reorder window"},
			} {
				rec := AppendRecord(nil, now, n, make([]byte, n))
				check("record", RecordSize(format, n, opt), func() error {
					return w.WriteRecord(0, rec, opt)
				})
			}
		}

		for _, s := range []struct {
			stats   Stats
			comment string
		}{
			{Stats{}, ""},
			{Stats{Start: now, End: now, Received: 4}, ""},
			{Stats{End: now}, "final"},
		} {
			check("stats", StatsSize(format, s.stats, s.comment), func() error {
				return w.WriteStats(0, s.stats, s.comment)
			})
		}

		for _, names := range [][]Name{
			nil,
			{{Addr: "bad"}, {Addr: "10.0.0.1"}},
			{{Addr: "10.0.0.1", Names: []string{"web"}}, {Addr: "fd00::1", Names: []string{"db", "db1"}}},
			{{Addr: "10.0.0.2", Names: []string{strings.Repeat("x", 0x10000)}}},
		} {
			check("names", NamesSize(format, names), func() error {
				return w.WriteNames(names)
			})
		}
	}
}
