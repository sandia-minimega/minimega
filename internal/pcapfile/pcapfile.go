// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

// Package pcapfile writes packet capture files for the minimega capture
// engine.
//
// Each packet is a record: a 16-byte header with ts_sec, ts_nsec, incl_len
// and orig_len as little-endian uint32 values, then incl_len bytes of frame
// data. A Writer buffers whole records and writes only whole records to the
// file, so a reader that follows a growing file never sees a partial record.
package pcapfile

import (
	"encoding/binary"
	"errors"
	"io"
	"time"
)

// RecordHeaderLen is the length of the header of a record.
const RecordHeaderLen = 16

// BufferSize is the size of the write buffer of a Writer.
const BufferSize = 1 << 20

// LinkTypeEthernet is the link type of every minimega capture.
const LinkTypeEthernet = 1

// ErrClosed is returned by a Writer after Close.
var ErrClosed = errors.New("pcapfile: writer is closed")

// ErrBadRecord is returned for a record that is too short or has an incl_len
// that does not match its data.
var ErrBadRecord = errors.New("pcapfile: malformed record")

// Interface describes one capture interface of a file.
type Interface struct {
	Name        string
	Description string
	Filter      string
	SnapLen     uint32
	Comment     string
}

// RecordOptions are optional values for one record.
type RecordOptions struct {
	Inbound, Outbound bool
	DropsBefore       uint64
	Comment           string
}

// Stats are the counters of one interface.
type Stats struct {
	Start, End time.Time
	Received   uint64 // frames that the interface received
	IfDrop     uint64 // frames that the kernel or the path dropped
	OSDrop     uint64 // frames that minimega dropped
	Delivered  uint64 // frames that are in the file
}

// Name is one address and its names, for a name resolution block.
type Name struct {
	Addr  string
	Names []string
}

// Writer writes packet records to a capture file.
type Writer interface {
	// AddInterface adds an interface and returns its ID.
	AddInterface(Interface) (uint32, error)
	// WriteRecord writes one record on interface iface.
	WriteRecord(iface uint32, rec []byte, opt RecordOptions) error
	// WriteStats writes the counters of interface iface.
	WriteStats(iface uint32, s Stats, comment string) error
	// WriteNames writes addresses and their names.
	WriteNames([]Name) error
	// Flush writes the buffered records to the file.
	Flush() error
	// Close flushes the writer. It does not close the file.
	Close() error
	// Size returns the bytes written so far, buffered bytes included.
	Size() int64
	// Format returns the name of the file format.
	Format() string
}

// AppendRecord appends a record for data, captured at ts from a frame of
// origLen bytes, to dst.
func AppendRecord(dst []byte, ts time.Time, origLen int, data []byte) []byte {
	var h [RecordHeaderLen]byte

	binary.LittleEndian.PutUint32(h[0:], uint32(ts.Unix()))
	binary.LittleEndian.PutUint32(h[4:], uint32(ts.Nanosecond()))
	binary.LittleEndian.PutUint32(h[8:], uint32(len(data)))
	binary.LittleEndian.PutUint32(h[12:], uint32(origLen))

	dst = append(dst, h[:]...)
	return append(dst, data...)
}

// RecordHeader is the decoded header of a record.
type RecordHeader struct {
	Time    time.Time
	InclLen uint32
	OrigLen uint32
}

// ParseRecordHeader decodes the header of the record at the start of b.
func ParseRecordHeader(b []byte) (RecordHeader, error) {
	if len(b) < RecordHeaderLen {
		return RecordHeader{}, ErrBadRecord
	}

	sec := binary.LittleEndian.Uint32(b[0:])
	nsec := binary.LittleEndian.Uint32(b[4:])
	if nsec >= 1e9 {
		return RecordHeader{}, ErrBadRecord
	}

	return RecordHeader{
		Time:    time.Unix(int64(sec), int64(nsec)),
		InclLen: binary.LittleEndian.Uint32(b[8:]),
		OrigLen: binary.LittleEndian.Uint32(b[12:]),
	}, nil
}

// checkRecord returns the header of rec, or ErrBadRecord when the length of
// rec does not match its incl_len.
func checkRecord(rec []byte) (RecordHeader, error) {
	h, err := ParseRecordHeader(rec)
	if err != nil {
		return h, err
	}

	if int(h.InclLen) != len(rec)-RecordHeaderLen || h.InclLen > h.OrigLen {
		return h, ErrBadRecord
	}

	return h, nil
}

// buffer collects whole blocks and writes them to w.
type buffer struct {
	w      io.Writer
	buf    []byte
	size   int64
	closed bool
	err    error
}

func newBuffer(w io.Writer) buffer {
	return buffer{w: w, buf: make([]byte, 0, BufferSize)}
}

// reserve makes room for n more bytes, and flushes when they do not fit.
func (b *buffer) reserve(n int) error {
	if b.closed {
		return ErrClosed
	}
	if b.err != nil {
		return b.err
	}

	if len(b.buf)+n > cap(b.buf) {
		return b.flush()
	}

	return nil
}

func (b *buffer) write(p ...[]byte) {
	for _, v := range p {
		b.buf = append(b.buf, v...)
		b.size += int64(len(v))
	}
}

func (b *buffer) flush() error {
	if b.err != nil {
		return b.err
	}
	if len(b.buf) == 0 {
		return nil
	}

	_, err := b.w.Write(b.buf)
	b.buf = b.buf[:0]
	if err != nil {
		b.err = err
	}

	return err
}

func (b *buffer) close() error {
	if b.closed {
		return nil
	}

	err := b.flush()
	b.closed = true

	return err
}
