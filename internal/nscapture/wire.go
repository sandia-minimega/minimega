// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

// Package nscapture captures the edge traffic of a namespace on many hosts and
// merges it into one file on the head node.
//
// Each member host reads its edge mirrors and sends batches of packet records
// to the head over meshage. The head orders the records of all streams by
// timestamp, within a reorder window, and writes them to one pcap or pcapng
// file. The package uses no minimega globals: the transport, the clock and the
// packet sources are interfaces.
package nscapture

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
)

// Record formats of a Batch.
const (
	FormatClassic uint8 = 1 // 16-byte record headers
	FormatNg      uint8 = 2 // 24-byte record headers with port and drops
)

// ngRecordExtra is the length of the fields that a pcapng record adds after
// the 16-byte header: port index and drops_before, each a uint32.
const ngRecordExtra = 8

// Batch carries the packet records of one stream (one host and bridge) from a
// member to the head.
type Batch struct {
	Namespace, Head string
	ID              int    // capture ID on the head
	Token           uint64 // random per capture start
	Host, Bridge    string // Bridge "" for the host-level final batch
	Seq             uint64 // per (Host, Bridge), from 1
	Through         int64  // unix ns: all records at or before it are sent
	SentAt          int64  // member clock at send
	Packets         uint32
	Data            []byte // records

	Captured, CapturedBytes uint64 // cumulative
	KernelDrops, IfDrops    uint64
	PathDrops               uint64 // estimate
	QueueDrops, SendDrops   uint64
	Ports                   int
	SkewNS                  int64
	Final                   bool
	Err                     string

	Format    uint8
	PortTable []PortInfo // pcapng: in the first batch and when it changes
}

// PortInfo describes one edge port of a stream, for pcapng interfaces and
// names.
type PortInfo struct {
	Index   uint32 // from 1; 0 means the stream itself
	Port    string // OVS port name
	MAC     string
	VLAN    int
	Kind    string // vm, container or host
	VM      string // empty for host taps
	VMIface int
	Addrs   []string
}

// ControlOp is the operation of a Control message.
type ControlOp int

const (
	OpAck ControlOp = iota + 1
	OpStop
)

// Control carries acks and stops from the head to a member.
type Control struct {
	Op                 ControlOp
	Namespace, Head    string
	ID                 int
	Token              uint64
	Bridge             string // OpAck
	Seq                uint64 // OpAck: highest seq received in order
	Window             int    // OpAck: batches allowed in flight
	HeadRecv, HeadSend int64  // OpAck: for the skew estimate
	Unknown            bool   // OpAck: head does not know this capture
}

// ErrMalformed is returned for a batch with records that are not valid.
var ErrMalformed = errors.New("malformed batch")

// recordHeaderLen returns the record header length for a batch format.
func recordHeaderLen(format uint8) int {
	if format == FormatNg {
		return pcapfile.RecordHeaderLen + ngRecordExtra
	}

	return pcapfile.RecordHeaderLen
}

// record is one record in the data of a batch.
type record struct {
	ts      int64  // unix ns
	rec     []byte // classic record: 16-byte header and data
	port    uint32 // pcapng port index
	dropsB4 uint32
}

// splitRecords validates the records in data and returns them. Each record
// must have incl_len <= snaplen and incl_len <= orig_len, and the count must
// equal packets.
func splitRecords(data []byte, format uint8, packets uint32, snaplen uint32) ([]record, error) {
	if format != FormatClassic && format != FormatNg {
		return nil, fmt.Errorf("%w: unknown format %v", ErrMalformed, format)
	}

	hlen := recordHeaderLen(format)
	res := make([]record, 0, min(int(packets), len(data)/hlen))

	for len(data) > 0 {
		if len(data) < hlen {
			return nil, fmt.Errorf("%w: short record header", ErrMalformed)
		}

		h, err := pcapfile.ParseRecordHeader(data)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
		}

		if h.InclLen > h.OrigLen || (snaplen > 0 && h.InclLen > snaplen) {
			return nil, fmt.Errorf("%w: record length %v", ErrMalformed, h.InclLen)
		}

		end := hlen + int(h.InclLen)
		if end > len(data) {
			return nil, fmt.Errorf("%w: record data past the end", ErrMalformed)
		}

		r := record{ts: h.Time.UnixNano()}

		if format == FormatNg {
			r.port = binary.LittleEndian.Uint32(data[16:])
			r.dropsB4 = binary.LittleEndian.Uint32(data[20:])

			// rebuild a classic record: the header, then the data
			buf := make([]byte, 0, pcapfile.RecordHeaderLen+int(h.InclLen))
			buf = append(buf, data[:pcapfile.RecordHeaderLen]...)
			r.rec = append(buf, data[hlen:end]...)
		} else {
			r.rec = data[:end:end]
		}

		res = append(res, r)
		data = data[end:]

		if uint32(len(res)) > packets {
			return nil, fmt.Errorf("%w: more records than packets", ErrMalformed)
		}
	}

	if uint32(len(res)) != packets {
		return nil, fmt.Errorf("%w: %v records, want %v", ErrMalformed, len(res), packets)
	}

	return res, nil
}

// appendNgRecord appends a pcapng batch record: the classic record rec with
// its port index and drops before it.
func appendNgRecord(dst, rec []byte, port, dropsBefore uint32) []byte {
	dst = append(dst, rec[:pcapfile.RecordHeaderLen]...)
	dst = binary.LittleEndian.AppendUint32(dst, port)
	dst = binary.LittleEndian.AppendUint32(dst, dropsBefore)

	return append(dst, rec[pcapfile.RecordHeaderLen:]...)
}
