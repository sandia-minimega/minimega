package gonetflow

import (
	"compress/gzip"
	"fmt"
	log "minilog"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	TCP                = 6
	UDP                = 17
	NETFLOW_HEADER_LEN = 24
	NETFLOW_RECORD_LEN = 48
	BUFFER_DEPTH       = 1024
	UDP_BUFFER_DEPTH   = 65536
)

const (
	ASCII = iota
	RAW
)

type Netflow struct {
	conn        *net.UDPConn
	writers     map[string]chan *Packet
	statBytes   uint64
	statRecords uint64
}

type Packet struct {
	Header  *Header
	Records []*Record
	Raw     []byte
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
func NewNetflow() (*Netflow, int, error) {
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

	return nf, p, nil
}

// stop and exit the reader goroutine for this object
func (nf *Netflow) Stop() {
	for k, _ := range nf.writers {
		nf.unregisterWriter(k)
	}
	nf.conn.Close()
}

func (nf *Netflow) GetStats() (uint64, uint64) {
	return nf.statBytes, nf.statRecords
}

func (nf *Netflow) NewSocketWriter(network string, server string, mode int) error {
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

func (nf *Netflow) NewFileWriter(filename string, mode int, compress bool) error {
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
	if _, ok := nf.writers[path]; !ok {
		return fmt.Errorf("netflow writer %v does not exist", path)
	}
	nf.unregisterWriter(path)
	return nil
}

func (nf *Netflow) unregisterWriter(path string) {
	close(nf.writers[path])
	delete(nf.writers, path)
}

func (nf *Netflow) registerWriter(path string, c chan *Packet) {
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
	if (n-NETFLOW_HEADER_LEN)%NETFLOW_RECORD_LEN != 0 {
		return nil, fmt.Errorf("invalid packet size %v", n)
	}
	numRecords := (n - NETFLOW_HEADER_LEN) / NETFLOW_RECORD_LEN

	p := &Packet{
		Header: &Header{
			Version:  int(b[1]), // skip the first byte
			Count:    int(b[3]),
			Sequence: (int32(b[16]) << 24) + (int32(b[17]) << 16) + (int32(b[18]) << 8) + (int32(b[19])),
		},
		Raw: b,
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

	nf.statBytes += uint64(n)
	nf.statRecords += uint64(len(p.Records))

	return p, nil
}
