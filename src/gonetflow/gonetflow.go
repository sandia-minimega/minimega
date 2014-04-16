package gonetflow

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	TCP                = 6
	UDP                = 17
	NETFLOW_HEADER_LEN = 24
	NETFLOW_RECORD_LEN = 48
)

type Netflow struct {
	Port int
	conn *net.UDPConn
}

type Packet struct {
	Header  *Header
	Records []*Record
}

type Header struct {
	Version int
	Count   int
	// Uptime time.Time
	// EpochSeconds time.Time
	// EpochNanoSeconds time.Time
	Sequence int32
}

type Record struct {
	Src        net.IP
	Dst        net.IP
	Nexthop    net.IP
	Input      int
	Output     int
	NumPackets int32
	NumOctets  int32
	SrcPort    int
	DstPort    int
	Protocol   int
	ToS        int
	SrcAS      int
	DstAS      int
}

func (p Packet) GoString() string {
	var ret string
	ret = fmt.Sprintf("Version: %v\nCount: %v\nSequence: %v\n", p.Header.Version, p.Header.Count, p.Header.Sequence)
	for _, r := range p.Records {
		ret += fmt.Sprintf("\tSrc: %v\n\tDst: %v\n\n", r.Src, r.Dst)
	}
	return ret
}

// NewNetflow returns a netflow object listening on port Netflow.Port
func NewNetflow() (*Netflow, error) {
	nf := &Netflow{}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	nf.conn = conn

	addr := nf.conn.LocalAddr()
	f := strings.SplitAfter(addr.String(), ":")
	if len(f) < 2 {
		return nil, fmt.Errorf("invalid LocalAddr %v", addr)
	}
	p, err := strconv.Atoi(f[len(f)-1])
	if err != nil {
		return nil, err
	}
	nf.Port = p

	go nf.reader()

	return nf, nil
}

func (nf *Netflow) reader() {
	var b = make([]byte, 1024)
	for {
		n, _, err := nf.conn.ReadFromUDP(b)
		if err != nil {
			fmt.Println(err)
		}
		nf.process(n, b)
	}
}

func (nf *Netflow) process(n int, b []byte) {
	if (n-NETFLOW_HEADER_LEN)%NETFLOW_RECORD_LEN != 0 {
		fmt.Printf("oops %v", n)
	}
	numRecords := (n - NETFLOW_HEADER_LEN) / NETFLOW_RECORD_LEN

	p := &Packet{
		Header: &Header{
			Version:  int(b[1]), // skip the first byte
			Count:    int(b[3]),
			Sequence: (int32(b[16]) << 24) + (int32(b[17]) << 16) + (int32(b[18]) << 8) + (int32(b[19])),
		},
	}

	for i := 0; i < numRecords; i++ {
		offset := (i * NETFLOW_RECORD_LEN) + NETFLOW_HEADER_LEN
		c := b[offset:]

		r := &Record{
			Src:        net.IP([]byte{c[0], c[1], c[2], c[3]}),
			Dst:        net.IP([]byte{c[4], c[5], c[6], c[7]}),
			Nexthop:    net.IP([]byte{c[8], c[9], c[10], c[11]}),
			Input:      (int(c[12]) << 8) + int(c[13]),
			Output:     (int(c[14]) << 8) + int(c[15]),
			NumPackets: (int32(c[16]) << 24) + (int32(c[17]) << 16) + (int32(c[18]) << 8) + (int32(c[19])),
			NumOctets:  (int32(c[20]) << 24) + (int32(c[21]) << 16) + (int32(c[22]) << 8) + (int32(c[23])),
			SrcPort:    (int(c[32]) << 8) + int(c[33]),
			DstPort:    (int(c[34]) << 8) + int(c[35]),
			Protocol:   int(c[38]),
			ToS:        int(c[39]),
			SrcAS:      (int(c[40]) << 8) + int(c[41]),
			DstAS:      (int(c[42]) << 8) + int(c[43]),
		}
		p.Records = append(p.Records, r)
	}

	fmt.Printf("got packet %#v", p)
}
