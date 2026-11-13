// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package pcapfile

import (
	"encoding/binary"
	"io"
)

// MagicNanoseconds is the magic number of a classic pcap file with nanosecond
// timestamps.
const MagicNanoseconds = 0xa1b23c4d

// FormatPcap is the name of the classic pcap format.
const FormatPcap = "pcap"

// classic writes classic pcap with nanosecond timestamps. It has one
// interface, so it ignores interfaces, statistics, names and comments.
type classic struct {
	buffer
}

// NewClassic writes the file header of a classic pcap file with nanosecond
// timestamps to w and returns a Writer for its records.
func NewClassic(w io.Writer, snaplen uint32) (Writer, error) {
	c := &classic{buffer: newBuffer(w)}

	var h [24]byte
	binary.LittleEndian.PutUint32(h[0:], MagicNanoseconds)
	binary.LittleEndian.PutUint16(h[4:], 2)
	binary.LittleEndian.PutUint16(h[6:], 4)
	binary.LittleEndian.PutUint32(h[16:], snaplen)
	binary.LittleEndian.PutUint32(h[20:], LinkTypeEthernet)

	c.write(h[:])

	// write the header at once so that a reader can identify the file
	if err := c.flush(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *classic) Format() string {
	return FormatPcap
}

func (c *classic) AddInterface(Interface) (uint32, error) {
	return 0, nil
}

func (c *classic) WriteRecord(_ uint32, rec []byte, _ RecordOptions) error {
	if _, err := checkRecord(rec); err != nil {
		return err
	}

	if err := c.reserve(len(rec)); err != nil {
		return err
	}

	c.write(rec)

	return nil
}

func (c *classic) WriteStats(uint32, Stats, string) error {
	return nil
}

func (c *classic) WriteNames([]Name) error {
	return nil
}

func (c *classic) Flush() error {
	return c.flush()
}

func (c *classic) Close() error {
	return c.close()
}

func (c *classic) Size() int64 {
	return c.size
}
