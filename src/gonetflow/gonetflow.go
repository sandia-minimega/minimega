// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package gonetflow

import (
	"bytes"
	"compress/gzip"
	"fmt"
	log "minilog"
	"net"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	TCP                = 6
	UDP                = 17
	NETFLOW_HEADER_LEN = 24
	NETFLOW_RECORD_LEN = 48
	BUFFER_DEPTH       = 1024
	UDP_BUFFER_DEPTH   = 65536
)

type Mode int

const (
	RAW Mode = iota
	ASCII
)

type Netflow struct {
	port          int
	conn          *net.UDPConn
	writers       map[string]chan *Packet
	statFlows     uint64
	statBytes     uint64
	statPackets   uint64
	statNFBytes   uint64
	statNFRecords uint64
}

type Packet struct {
	Header  *Header
	Records []*Record
	Raw     []byte
}

type Header struct {
	Version   int
	Count     int
	Uptime    uint32
	EpochSec  uint32
	EpochNsec uint32
	Sequence  int32
}

type Record struct {
	Src        net.IP
	Dst        net.IP
	Nexthop    net.IP
	Input      uint
	Output     uint
	NumPackets uint32
	NumOctets  uint32
	First      uint32
	Last       uint32
	SrcPort    int
	DstPort    int
	Protocol   int
	ToS        int
	SrcAS      int
	DstAS      int
}

func (m Mode) String() string {
	switch m {
	case ASCII:
		return "ascii"
	case RAW:
		return "raw"
	}

	return "???"
}

func (p Packet) GoString() string {
	var ret string
	for _, r := range p.Records {
		offsetFirst := int64(p.Header.Uptime) - int64(r.First)
		offsetLast := int64(p.Header.Uptime) - int64(r.Last)
		f := (((int64(p.Header.EpochSec) * 1000) - offsetFirst) * 1000000) + int64(p.Header.EpochNsec)
		l := (((int64(p.Header.EpochSec) * 1000) - offsetLast) * 1000000) + int64(p.Header.EpochNsec)
		start := time.Unix(0, f)
		stop := time.Unix(0, l)
		ret += fmt.Sprintf("%v\t%v\t%v\t%v:%v\t<->\t%v:%v\t%v\t%v\n", start.Format(time.RFC3339), stop.Sub(start), r.Protocol, r.Src, r.SrcPort, r.Dst, r.DstPort, r.NumPackets, r.NumOctets)
	}
	return ret
}

// NewNetflow returns a netflow object listening on port Netflow.Port
func NewNetflow() (*Netflow, int, error) {
	log.Debugln("NewNetflow")
	nf := &Netflow{
		writers: make(map[string]chan *Packet),
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		return nil, -1, err
	}
	nf.conn = conn

	addr := nf.conn.LocalAddr()
	f := strings.SplitAfter(addr.String(), ":")
	if len(f) < 2 {
		return nil, -1, fmt.Errorf("invalid LocalAddr %v", addr)
	}
	p, err := strconv.Atoi(f[len(f)-1])
	if err != nil {
		return nil, -1, err
	}

	go nf.reader()

	nf.port = p

	return nf, p, nil
}

func (nf *Netflow) GetPort() int {
	return nf.port
}

// stop and exit the reader goroutine for this object
func (nf *Netflow) Stop() {
	log.Debugln("Stop")
	for k, _ := range nf.writers {
		nf.unregisterWriter(k)
	}
	nf.conn.Close()
}

// Date flow start          Duration Proto      Src IP Addr:Port           Dst IP Addr:Port   Out Pkt   In Pkt Out Byte  In Byte Flows
// Summary: total flows: 290, total bytes: 24.5 G, total packets: 17.1 M, avg bps: 1493, avg pps: 0, avg bpp: 1436
// Time window: 2014-04-20 16:43:25 - 2010-04-13 01:09:07
// Total flows processed: 290, Blocks skipped: 0, Bytes read: 15108
// Sys: 0.000s flows/second: 0.0        Wall: 0.000s flows/second: 665137.6

func (nf *Netflow) GetStats() string {
	log.Debugln("GetStats")

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Netflow summary:")
	fmt.Fprintf(w, "\tTotal flows:\t%v\n", nf.statFlows)
	fmt.Fprintf(w, "\tTotal bytes:\t%v\n", nf.statBytes)
	fmt.Fprintf(w, "\tTotal packets:\t%v\n", nf.statPackets)
	fmt.Fprintf(w, "\tTotal netflow monitor bytes:\t%v\n", nf.statNFBytes)
	fmt.Fprintf(w, "\tTotal netflow monitor records:\t%v\n", nf.statNFRecords)
	w.Flush()
	return o.String()
}

func (nf *Netflow) NewSocketWriter(network string, server string, mode Mode) error {
	log.Debugln("NewSocketWriter")
	if _, ok := nf.writers[server]; ok {
		return fmt.Errorf("netflow writer %v already exists", server)
	}

	conn, err := net.Dial(network, server)
	if err != nil {
		return err
	}

	c := make(chan *Packet, BUFFER_DEPTH)
	go func() {
		for {
			d := <-c
			if d == nil {
				break
			}
			if mode == ASCII {
				conn.Write([]byte(d.GoString()))
			} else {
				conn.Write(d.Raw)
			}
		}
		conn.Close()
	}()

	name := fmt.Sprintf("%v:%v", network, server)
	nf.registerWriter(name, c)
	return nil
}

func (nf *Netflow) NewFileWriter(filename string, mode Mode, compress bool) error {
	log.Debugln("NewFileWriter")
	if _, ok := nf.writers[filename]; ok {
		return fmt.Errorf("netflow writer %v already exists", filename)
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	c := make(chan *Packet, BUFFER_DEPTH)
	go func() {
		var w *gzip.Writer
		if compress {
			log.Debugln("using compression")
			w = gzip.NewWriter(f)
		}
		for {
			d := <-c
			if d == nil {
				break
			}
			if mode == ASCII {
				if compress {
					w.Write([]byte(d.GoString()))
				} else {
					f.Write([]byte(d.GoString()))
				}
			} else {
				if compress {
					w.Write(d.Raw)
				} else {
					f.Write(d.Raw)
				}
			}
		}
		if compress {
			w.Close()
		}
		f.Close()
	}()

	nf.registerWriter(filename, c)
	return nil
}

func (nf *Netflow) RemoveWriter(path string) error {
	log.Debug("RemoveWriter %v", path)
	if _, ok := nf.writers[path]; !ok {
		return fmt.Errorf("netflow writer %v does not exist", path)
	}
	nf.unregisterWriter(path)
	return nil
}

// HasWriter returns true if we have any writers
func (nf *Netflow) HasWriter() bool {
	return len(nf.writers) > 0
}

func (nf *Netflow) unregisterWriter(path string) {
	log.Debug("unregisterWriter %v", path)
	close(nf.writers[path])
	delete(nf.writers, path)
}

func (nf *Netflow) registerWriter(path string, c chan *Packet) {
	log.Debug("registerWriter %v", path)
	nf.writers[path] = c
}

func (nf *Netflow) reader() {
	var b = make([]byte, UDP_BUFFER_DEPTH)
	for {
		n, _, err := nf.conn.ReadFromUDP(b)
		if err != nil {
			if strings.Contains(err.Error(), "closed") {
				return
			}
			log.Errorln(err)
			continue
		}
		p, err := nf.process(n, b)
		if err != nil {
			log.Errorln(err)
			continue
		}
		for _, v := range nf.writers {
			v <- p
		}
	}
}

func (nf *Netflow) process(n int, b []byte) (*Packet, error) {
	if int(b[1]) != 5 {
		return nil, fmt.Errorf("invalid netflow record version %v", int(b[1]))
	}
	if (n-NETFLOW_HEADER_LEN)%NETFLOW_RECORD_LEN != 0 {
		return nil, fmt.Errorf("invalid packet size %v", n)
	}
	numRecords := (n - NETFLOW_HEADER_LEN) / NETFLOW_RECORD_LEN

	p := &Packet{
		Header: DecodeHeader(b),
		Raw:    b[:n],
	}

	for i := 0; i < numRecords; i++ {
		offset := (i * NETFLOW_RECORD_LEN) + NETFLOW_HEADER_LEN
		c := b[offset:]

		r := DecodeRecord(c)
		p.Records = append(p.Records, r)

		nf.statBytes += uint64(r.NumOctets)
		nf.statPackets += uint64(r.NumPackets)
	}

	nf.statFlows += uint64(p.Header.Count)
	nf.statNFBytes += uint64(n)
	nf.statNFRecords += uint64(len(p.Records))

	return p, nil
}

func DecodeHeader(b []byte) *Header {
	return &Header{
		Version:   int(b[0])<<8 + int(b[1]),
		Count:     int(b[2])<<8 + int(b[3]),
		Sequence:  (int32(b[16]) << 24) + (int32(b[17]) << 16) + (int32(b[18]) << 8) + (int32(b[19])),
		Uptime:    (uint32(b[4]) << 24) + (uint32(b[5]) << 16) + (uint32(b[6]) << 8) + (uint32(b[7])),
		EpochSec:  (uint32(b[8]) << 24) + (uint32(b[9]) << 16) + (uint32(b[10]) << 8) + (uint32(b[11])),
		EpochNsec: (uint32(b[12]) << 24) + (uint32(b[13]) << 16) + (uint32(b[14]) << 8) + (uint32(b[15])),
	}
}

func DecodeRecord(b []byte) *Record {
	return &Record{
		Src:        net.IP([]byte{b[0], b[1], b[2], b[3]}),
		Dst:        net.IP([]byte{b[4], b[5], b[6], b[7]}),
		Nexthop:    net.IP([]byte{b[8], b[9], b[10], b[11]}),
		Input:      (uint(b[12]) << 8) + uint(b[13]),
		Output:     (uint(b[14]) << 8) + uint(b[15]),
		NumPackets: (uint32(b[16]) << 24) + (uint32(b[17]) << 16) + (uint32(b[18]) << 8) + (uint32(b[19])),
		NumOctets:  (uint32(b[20]) << 24) + (uint32(b[21]) << 16) + (uint32(b[22]) << 8) + (uint32(b[23])),
		First:      (uint32(b[24]) << 24) + (uint32(b[25]) << 16) + (uint32(b[26]) << 8) + (uint32(b[27])),
		Last:       (uint32(b[28]) << 24) + (uint32(b[29]) << 16) + (uint32(b[30]) << 8) + (uint32(b[31])),
		SrcPort:    (int(b[32]) << 8) + int(b[33]),
		DstPort:    (int(b[34]) << 8) + int(b[35]),
		Protocol:   int(b[38]),
		ToS:        int(b[39]),
		SrcAS:      (int(b[40]) << 8) + int(b[41]),
		DstAS:      (int(b[42]) << 8) + int(b[43]),
	}
}
