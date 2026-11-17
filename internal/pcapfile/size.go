// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package pcapfile

import "net/netip"

// The functions in this file return the bytes that a Writer of a format adds
// to a file, so that a caller can keep a file below a maximum size. Classic
// pcap writes only records.

// ngOptSize returns the size of an option with a value of n bytes.
func ngOptSize(n int) int {
	n = min(n, ngMaxValue)
	return 4 + n + pad4(n)
}

// ngBlockSize returns the size of a block with a fixed body of fixed bytes,
// data bytes, and opts bytes of options that end with opt_endofopt.
func ngBlockSize(fixed, data, opts int) int {
	if opts > 0 {
		opts += len(zeros)
	}

	return 12 + fixed + data + pad4(data) + opts
}

// RecordSize returns the bytes that WriteRecord adds for a record with
// dataLen bytes of frame data and the options opt.
func RecordSize(format string, dataLen int, opt RecordOptions) int {
	if format != FormatPcapng {
		return RecordHeaderLen + dataLen
	}

	var opts int
	if opt.Inbound || opt.Outbound {
		opts += ngOptSize(4)
	}
	if opt.DropsBefore > 0 {
		opts += ngOptSize(8)
	}
	if opt.Comment != "" {
		opts += ngOptSize(len(opt.Comment))
	}

	return ngBlockSize(20, dataLen, opts)
}

// InterfaceSize returns the bytes that AddInterface adds for i.
func InterfaceSize(format string, i Interface) int {
	if format != FormatPcapng {
		return 0
	}

	opts := ngOptSize(1) // if_tsresol
	if i.Name != "" {
		opts += ngOptSize(len(i.Name))
	}
	if i.Description != "" {
		opts += ngOptSize(len(i.Description))
	}
	if i.Filter != "" {
		opts += ngOptSize(1 + len(i.Filter))
	}
	if i.Comment != "" {
		opts += ngOptSize(len(i.Comment))
	}

	return ngBlockSize(8, 0, opts)
}

// StatsSize returns the bytes that WriteStats adds for s and comment.
func StatsSize(format string, s Stats, comment string) int {
	if format != FormatPcapng {
		return 0
	}

	opts := 4 * ngOptSize(8) // the four counters
	if !s.Start.IsZero() {
		opts += ngOptSize(8)
	}
	if !s.End.IsZero() {
		opts += ngOptSize(8)
	}
	if comment != "" {
		opts += ngOptSize(len(comment))
	}

	return ngBlockSize(12, 0, opts)
}

// NamesSize returns the bytes that WriteNames adds for names.
func NamesSize(format string, names []Name) int {
	if format != FormatPcapng {
		return 0
	}

	var opts int
	for _, nm := range names {
		a, err := netip.ParseAddr(nm.Addr)
		if err != nil || len(nm.Names) == 0 {
			continue
		}

		n := len(a.AsSlice())
		for _, s := range nm.Names {
			n += len(s) + 1
		}
		if n > ngMaxValue {
			continue
		}

		opts += ngOptSize(n)
	}

	if opts == 0 {
		return 0
	}

	// the record list ends with an end record, not with opt_endofopt
	return 12 + opts + 4
}
