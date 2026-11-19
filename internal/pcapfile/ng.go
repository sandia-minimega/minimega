// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package pcapfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"time"
)

// FormatPcapng is the name of the pcapng format.
const FormatPcapng = "pcapng"

const (
	ngBlockSHB = 0x0A0D0D0A
	ngBlockIDB = 0x00000001
	ngBlockNRB = 0x00000004
	ngBlockISB = 0x00000005
	ngBlockEPB = 0x00000006

	ngByteOrderMagic = 0x1A2B3C4D

	ngOptEnd     = 0
	ngOptComment = 1

	ngOptSHBHardware = 2
	ngOptSHBOS       = 3
	ngOptSHBUserAppl = 4

	ngOptIfName        = 2
	ngOptIfDescription = 3
	ngOptIfTsresol     = 9
	ngOptIfFilter      = 11

	ngOptEPBFlags     = 2
	ngOptEPBDropCount = 4

	ngOptISBStartTime = 2
	ngOptISBEndTime   = 3
	ngOptISBIfRecv    = 4
	ngOptISBIfDrop    = 5
	ngOptISBOSDrop    = 7
	ngOptISBUsrDeliv  = 8

	ngRecordEnd  = 0
	ngRecordIPv4 = 1
	ngRecordIPv6 = 2

	ngMaxValue = 0xFFFF
)

var errUnknownInterface = errors.New("pcapfile: unknown interface")

var zeros [4]byte

// Section holds the values of the section header of a pcapng file. Empty
// values are not written.
type Section struct {
	UserAppl string
	OS       string
	Hardware string
	Comment  string
}

// ng writes pcapng with nanosecond timestamps on all interfaces.
type ng struct {
	buffer
	ifaces uint32
	opts   []byte
	fixed  [28]byte
}

// NewNg writes the section header block of a pcapng file to w and returns a
// Writer for its blocks.
func NewNg(w io.Writer, s Section) (Writer, error) {
	n := &ng{buffer: newBuffer(w)}

	n.opts = n.opts[:0]
	n.addString(ngOptSHBHardware, s.Hardware)
	n.addString(ngOptSHBOS, s.OS)
	n.addString(ngOptSHBUserAppl, s.UserAppl)
	n.addString(ngOptComment, s.Comment)
	n.opts = append(n.opts, zeros[:]...)

	f := n.fixed[:16]
	binary.LittleEndian.PutUint32(f[0:], ngByteOrderMagic)
	binary.LittleEndian.PutUint16(f[4:], 1)
	binary.LittleEndian.PutUint16(f[6:], 0)
	binary.LittleEndian.PutUint64(f[8:], ^uint64(0))

	if err := n.block(ngBlockSHB, f, nil, false); err != nil {
		return nil, err
	}

	// write the header at once so that a reader can identify the file
	if err := n.flush(); err != nil {
		return nil, err
	}

	return n, nil
}

func pad4(n int) int {
	return (4 - n&3) & 3
}

// add appends an option to n.opts. It cuts values that are too long.
func (n *ng) add(code uint16, v []byte) {
	if len(v) > ngMaxValue {
		v = v[:ngMaxValue]
	}

	var h [4]byte
	binary.LittleEndian.PutUint16(h[0:], code)
	binary.LittleEndian.PutUint16(h[2:], uint16(len(v)))

	n.opts = append(n.opts, h[:]...)
	n.opts = append(n.opts, v...)
	n.opts = append(n.opts, zeros[:pad4(len(v))]...)
}

func (n *ng) addString(code uint16, s string) {
	if s != "" {
		n.add(code, []byte(s))
	}
}

func (n *ng) addUint64(code uint16, v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	n.add(code, b[:])
}

// addTime appends a timestamp option: high 32 bits, then low 32 bits.
func (n *ng) addTime(code uint16, t time.Time) {
	var b [8]byte
	putTime(b[:], t)
	n.add(code, b[:])
}

func putTime(b []byte, t time.Time) {
	ns := uint64(t.UnixNano())
	binary.LittleEndian.PutUint32(b[0:], uint32(ns>>32))
	binary.LittleEndian.PutUint32(b[4:], uint32(ns))
}

// block writes one block with the fixed body, data padded to 4 bytes, and
// n.opts. When end is true and n.opts is not empty, it adds opt_endofopt.
func (n *ng) block(typ uint32, fixed, data []byte, end bool) error {
	if end && len(n.opts) > 0 {
		n.opts = append(n.opts, zeros[:]...)
	}

	pad := pad4(len(data))
	total := 12 + len(fixed) + len(data) + pad + len(n.opts)

	if err := n.reserve(total); err != nil {
		return err
	}

	var h, t [8]byte
	binary.LittleEndian.PutUint32(h[0:], typ)
	binary.LittleEndian.PutUint32(h[4:], uint32(total))
	binary.LittleEndian.PutUint32(t[0:], uint32(total))

	n.write(h[:], fixed, data, zeros[:pad], n.opts, t[:4])

	return nil
}

func (n *ng) checkInterface(iface uint32) error {
	if n.closed {
		return ErrClosed
	}
	if iface >= n.ifaces {
		return fmt.Errorf("%w: %d", errUnknownInterface, iface)
	}

	return nil
}

func (n *ng) Format() string {
	return FormatPcapng
}

// AddInterface writes an interface description block. The interface uses
// Ethernet and nanosecond timestamps.
func (n *ng) AddInterface(i Interface) (uint32, error) {
	n.opts = n.opts[:0]
	n.addString(ngOptIfName, i.Name)
	n.addString(ngOptIfDescription, i.Description)
	n.add(ngOptIfTsresol, []byte{9})
	if i.Filter != "" {
		n.add(ngOptIfFilter, append([]byte{0}, i.Filter...))
	}
	n.addString(ngOptComment, i.Comment)

	f := n.fixed[:8]
	binary.LittleEndian.PutUint16(f[0:], LinkTypeEthernet)
	binary.LittleEndian.PutUint16(f[2:], 0)
	binary.LittleEndian.PutUint32(f[4:], i.SnapLen)

	if err := n.block(ngBlockIDB, f, nil, true); err != nil {
		return 0, err
	}

	id := n.ifaces
	n.ifaces++

	return id, nil
}

// WriteRecord writes rec as an enhanced packet block.
func (n *ng) WriteRecord(iface uint32, rec []byte, opt RecordOptions) error {
	h, err := checkRecord(rec)
	if err != nil {
		return err
	}
	if err := n.checkInterface(iface); err != nil {
		return err
	}

	n.opts = n.opts[:0]
	if opt.Inbound || opt.Outbound {
		var flags [4]byte
		if opt.Inbound {
			flags[0] = 1
		} else {
			flags[0] = 2
		}
		n.add(ngOptEPBFlags, flags[:])
	}
	if opt.DropsBefore > 0 {
		n.addUint64(ngOptEPBDropCount, opt.DropsBefore)
	}
	n.addString(ngOptComment, opt.Comment)

	f := n.fixed[:20]
	binary.LittleEndian.PutUint32(f[0:], iface)
	putTime(f[4:], h.Time)
	binary.LittleEndian.PutUint32(f[12:], h.InclLen)
	binary.LittleEndian.PutUint32(f[16:], h.OrigLen)

	return n.block(ngBlockEPB, f, rec[RecordHeaderLen:], true)
}

// WriteStats writes an interface statistics block. The block time is s.End,
// or the current time when s.End is zero.
func (n *ng) WriteStats(iface uint32, s Stats, comment string) error {
	if err := n.checkInterface(iface); err != nil {
		return err
	}

	ts := s.End
	if ts.IsZero() {
		ts = time.Now()
	}

	n.opts = n.opts[:0]
	if !s.Start.IsZero() {
		n.addTime(ngOptISBStartTime, s.Start)
	}
	if !s.End.IsZero() {
		n.addTime(ngOptISBEndTime, s.End)
	}
	n.addUint64(ngOptISBIfRecv, s.Received)
	n.addUint64(ngOptISBIfDrop, s.IfDrop)
	n.addUint64(ngOptISBOSDrop, s.OSDrop)
	n.addUint64(ngOptISBUsrDeliv, s.Delivered)
	n.addString(ngOptComment, comment)

	f := n.fixed[:12]
	binary.LittleEndian.PutUint32(f[0:], iface)
	putTime(f[4:], ts)

	return n.block(ngBlockISB, f, nil, true)
}

// WriteNames writes a name resolution block. It skips names with an address
// that does not parse or with no names. It writes nothing when no names
// remain.
func (n *ng) WriteNames(names []Name) error {
	if n.closed {
		return ErrClosed
	}

	n.opts = n.opts[:0]
	for _, nm := range names {
		a, err := netip.ParseAddr(nm.Addr)
		if err != nil || len(nm.Names) == 0 {
			continue
		}

		code, addr := uint16(ngRecordIPv6), a.AsSlice()
		if a.Is4() {
			code = ngRecordIPv4
		}

		v := addr
		for _, s := range nm.Names {
			v = append(append(v, s...), 0)
		}
		if len(v) > ngMaxValue {
			continue
		}

		n.add(code, v)
	}

	if len(n.opts) == 0 {
		return nil
	}

	n.opts = append(n.opts, ngRecordEnd, 0, 0, 0)

	return n.block(ngBlockNRB, nil, nil, false)
}

func (n *ng) Flush() error {
	return n.flush()
}

func (n *ng) Close() error {
	return n.close()
}

func (n *ng) Size() int64 {
	return n.size
}
