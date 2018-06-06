// Package protocol implements the 9p protocol using the stubs.

//go:generate go run gen.go

package protocol

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync/atomic"
)

var serverprofile = flag.String("serverprofile", "", "This is for specifying the prefix of the output file for the profile")

// 9P2000 message types
const (
	Tversion MType = 100 + iota
	Rversion
	Tauth
	Rauth
	Tattach
	Rattach
	Terror
	Rerror
	Tflush
	Rflush
	Twalk
	Rwalk
	Topen
	Ropen
	Tcreate
	Rcreate
	Tread
	Rread
	Twrite
	Rwrite
	Tclunk
	Rclunk
	Tremove
	Rremove
	Tstat
	Rstat
	Twstat
	Rwstat
	Tlast
)

const (
	MSIZE   = 2*1048576 + IOHDRSZ // default message size (1048576+IOHdrSz)
	IOHDRSZ = 24                  // the non-data size of the Twrite messages
	PORT    = 564                 // default port for 9P file servers
	NumFID  = 1 << 16
	QIDLen  = 13
)

// QID types
const (
	QTDIR     = 0x80 // directories
	QTAPPEND  = 0x40 // append only files
	QTEXCL    = 0x20 // exclusive use files
	QTMOUNT   = 0x10 // mounted channel
	QTAUTH    = 0x08 // authentication file
	QTTMP     = 0x04 // non-backed-up file
	QTSYMLINK = 0x02 // symbolic link (Unix, 9P2000.u)
	QTLINK    = 0x01 // hard link (Unix, 9P2000.u)
	QTFILE    = 0x00
)

// Flags for the mode field in Topen and Tcreate messages
const (
	OREAD   = 0x0    // open read-only
	OWRITE  = 0x1    // open write-only
	ORDWR   = 0x2    // open read-write
	OEXEC   = 0x3    // execute (== read but check execute permission)
	OTRUNC  = 0x10   // or'ed in (except for exec), truncate file first
	OCEXEC  = 0x20   // or'ed in, close on exec
	ORCLOSE = 0x40   // or'ed in, remove on close
	OAPPEND = 0x80   // or'ed in, append only
	OEXCL   = 0x1000 // or'ed in, exclusive client use
)

// File modes
const (
	DMDIR    = 0x80000000 // mode bit for directories
	DMAPPEND = 0x40000000 // mode bit for append only files
	DMEXCL   = 0x20000000 // mode bit for exclusive use files
	DMMOUNT  = 0x10000000 // mode bit for mounted channel
	DMAUTH   = 0x08000000 // mode bit for authentication file
	DMTMP    = 0x04000000 // mode bit for non-backed-up file
	DMREAD   = 0x4        // mode bit for read permission
	DMWRITE  = 0x2        // mode bit for write permission
	DMEXEC   = 0x1        // mode bit for execute permission
)

const (
	NOTAG Tag = 0xFFFF     // no tag specified
	NOFID FID = 0xFFFFFFFF // no fid specified
	// We reserve tag NOTAG and tag 0. 0 is a troublesome value to pass
	// around, since it is also a default value and using it can hide errors
	// in the code.
	NumTags = 1<<16 - 2
)

// Error values
const (
	EPERM   = 1
	ENOENT  = 2
	EIO     = 5
	EACCES  = 13
	EEXIST  = 17
	ENOTDIR = 20
	EINVAL  = 22
)

// Types contained in 9p messages.
type (
	MType      uint8
	Mode       uint8
	NumEntries uint16
	Tag        uint16
	FID        uint32
	MaxSize    uint32
	Count      int32
	Perm       uint32
	Offset     uint64
	Data       []byte
	// Some []byte fields are encoded with a 16-bit length, e.g. stat data.
	// We use this type to tag such fields. The parameters are still []byte,
	// this was just the only way I could think of to make the stub generator do the right
	// thing.
	DataCnt16 byte // []byte with a 16-bit count.
)

// Error represents a 9P2000 error
type Error struct {
	Err string
}

// File identifier
type QID struct {
	Type    uint8  // type of the file (high 8 bits of the mode)
	Version uint32 // version number for the path
	Path    uint64 // server's unique identification of the file
}

// Dir describes a file
type Dir struct {
	Type    uint16
	Dev     uint32
	QID     QID    // file's QID
	Mode    uint32 // permissions and flags
	Atime   uint32 // last access time in seconds
	Mtime   uint32 // last modified time in seconds
	Length  uint64 // file length in bytes
	Name    string // file name
	User    string // owner name
	Group   string // group name
	ModUser string // name of the last user that modified the file
}

type Dispatcher func(s *Server, b *bytes.Buffer, t MType) error

// N.B. In all packets, the wire order is assumed to be the order in which you
// put struct members.
// In an earlier version of this code we got really fancy and made it so you
// could have identically named fields in the R and T packets. It's only an issue
// in a trivial number of packets so we place the burden on you, the user, to make
// the names different. Also, you can't name struct members with the same names as the
// type. Sorry. But it keeps gen.go so much simpler.

type TversionPkt struct {
	TMsize   MaxSize
	TVersion string
}

type RversionPkt struct {
	RMsize   MaxSize
	RVersion string
}

type TattachPkt struct {
	SFID  FID
	AFID  FID
	Uname string
	Aname string
}

type RattachPkt struct {
	QID QID
}

type TflushPkt struct {
	OTag Tag
}

type RflushPkt struct {
}

type TwalkPkt struct {
	SFID   FID
	NewFID FID
	Paths  []string
}

type RwalkPkt struct {
	QIDs []QID
}

type TopenPkt struct {
	OFID  FID
	Omode Mode
}

type RopenPkt struct {
	OQID   QID
	IOUnit MaxSize
}

type TcreatePkt struct {
	OFID       FID
	Name       string
	CreatePerm Perm
	Omode      Mode
}

type RcreatePkt struct {
	OQID   QID
	IOUnit MaxSize
}

type TclunkPkt struct {
	OFID FID
}

type RclunkPkt struct {
}

type TremovePkt struct {
	OFID FID
}

type RremovePkt struct {
}

type TstatPkt struct {
	OFID FID
}

type RstatPkt struct {
	B []DataCnt16
}

type TwstatPkt struct {
	OFID FID
	B    []DataCnt16
}

type RwstatPkt struct {
}

type TreadPkt struct {
	OFID FID
	Off  Offset
	Len  Count
}

type RreadPkt struct {
	Data []byte
}

type TwritePkt struct {
	OFID FID
	Off  Offset
	Data []byte
}

type RwritePkt struct {
	RLen Count
}

type RerrorPkt struct {
	Error string
}

type DirPkt struct {
	D Dir
}

type RPCCall struct {
	b     []byte
	Reply chan []byte
}

type RPCReply struct {
	b []byte
}

/* rpc servers */
type ClientOpt func(*Client) error
type ServerOpt func(*Server) error
type Tracer func(string, ...interface{})

// Client implements a 9p client. It has a chan containing all tags,
// a scalar FID which is incremented to provide new FIDS (all FIDS for a given
// client are unique), an array of MaxTag-2 RPC structs, a ReadWriteCloser
// for IO, and two channels for a server goroutine: one down which RPCalls are
// pushed and another from which RPCReplys return.
// Once a client is marked Dead all further requests to it will fail.
// The ToNet/FromNet are separate so we can use io.Pipe for testing.
type Client struct {
	Tags       chan Tag
	FID        uint64
	RPC        []*RPCCall
	ToNet      io.WriteCloser
	FromNet    io.ReadCloser
	FromClient chan *RPCCall
	FromServer chan *RPCReply
	Msize      uint32
	Dead       bool
	Trace      Tracer
}

// Server is a 9p server.
// For now it's extremely serial. But we will use a chan for replies to ensure that
// we can go to a more concurrent one later.
type Server struct {
	NS        NineServer
	D         Dispatcher
	Versioned bool
	FromNet   io.ReadCloser
	ToNet     io.WriteCloser
	Replies   chan RPCReply
	Trace     Tracer
	Dead      bool

	fprofile *os.File
}

type NineServer interface {
	Rversion(MaxSize, string) (MaxSize, string, error)
	Rattach(FID, FID, string, string) (QID, error)
	Rwalk(FID, FID, []string) ([]QID, error)
	Ropen(FID, Mode) (QID, MaxSize, error)
	Rcreate(FID, string, Perm, Mode) (QID, MaxSize, error)
	Rstat(FID) ([]byte, error)
	Rwstat(FID, []byte) error
	Rclunk(FID) error
	Rremove(FID) error
	Rread(FID, Offset, Count) ([]byte, error)
	Rwrite(FID, Offset, []byte) (Count, error)
	Rflush(Otag Tag) error
}

var (
	RPCNames = map[MType]string{
		Tversion: "Tversion",
		Rversion: "Rversion",
		Tauth:    "Tauth",
		Rauth:    "Rauth",
		Tattach:  "Tattach",
		Rattach:  "Rattach",
		Terror:   "Terror",
		Rerror:   "Rerror",
		Tflush:   "Tflush",
		Rflush:   "Rflush",
		Twalk:    "Twalk",
		Rwalk:    "Rwalk",
		Topen:    "Topen",
		Ropen:    "Ropen",
		Tcreate:  "Tcreate",
		Rcreate:  "Rcreate",
		Tread:    "Tread",
		Rread:    "Rread",
		Twrite:   "Twrite",
		Rwrite:   "Rwrite",
		Tclunk:   "Tclunk",
		Rclunk:   "Rclunk",
		Tremove:  "Tremove",
		Rremove:  "Rremove",
		Tstat:    "Tstat",
		Rstat:    "Rstat",
		Twstat:   "Twstat",
		Rwstat:   "Rwstat",
	}
)

// GetTag gets a tag to be used to identify a message.
func (c *Client) GetTag() Tag {
	t := <-c.Tags
	if false {
		runtime.SetFinalizer(&t, func(t *Tag) {
			c.Tags <- *t
		})
	}
	return t
}

// GetFID gets a fid to be used to identify a resource for a 9p client.
// For a given lifetime of a 9p client, FIDS are unique (i.e. not reused as in
// many 9p client libraries).
func (c *Client) GetFID() FID {
	return FID(atomic.AddUint64(&c.FID, 1))
}

func (c *Client) readNetPackets() {
	if c.FromNet == nil {
		if c.Trace != nil {
			c.Trace("c.FromNet is nil, marking dead")
		}
		c.Dead = true
		return
	}
	defer c.FromNet.Close()
	defer close(c.FromServer)
	if c.Trace != nil {
		c.Trace("Starting readNetPackets")
	}
	for !c.Dead {
		l := make([]byte, 7)
		if c.Trace != nil {
			c.Trace("Before read")
		}

		if n, err := c.FromNet.Read(l); err != nil || n < 7 {
			log.Printf("readNetPackets: short read: %v", err)
			c.Dead = true
			return
		}
		if c.Trace != nil {
			c.Trace("Server reads %v", l)
		}
		s := int64(l[0]) + int64(l[1])<<8 + int64(l[2])<<16 + int64(l[3])<<24
		b := bytes.NewBuffer(l)
		r := io.LimitReader(c.FromNet, s-7)
		if _, err := io.Copy(b, r); err != nil {
			log.Printf("readNetPackets: short read: %v", err)
			c.Dead = true
			return
		}
		if c.Trace != nil {
			c.Trace("readNetPackets: got %v, len %d, sending to IO", RPCNames[MType(l[4])], b.Len())
		}
		c.FromServer <- &RPCReply{b: b.Bytes()}
	}
	if c.Trace != nil {
		c.Trace("Client %v is all done", c)
	}

}

func (c *Client) IO() {
	go func() {
		for {
			r := <-c.FromClient
			t := <-c.Tags
			if c.Trace != nil {
				c.Trace(fmt.Sprintf("Tag for request is %v", t))
			}
			r.b[5] = uint8(t)
			r.b[6] = uint8(t >> 8)
			if c.Trace != nil {
				c.Trace(fmt.Sprintf("Tag for request is %v", t))
			}
			c.RPC[int(t)-1] = r
			if c.Trace != nil {
				c.Trace("Write %v to ToNet", r.b)
			}
			if _, err := c.ToNet.Write(r.b); err != nil {
				c.Dead = true
				log.Fatalf("Write to server: %v", err)
				return
			}
		}
	}()

	for {
		r := <-c.FromServer
		if c.Trace != nil {
			c.Trace("Read %v FromServer", r.b)
		}
		t := Tag(r.b[5]) | Tag(r.b[6])<<8
		if c.Trace != nil {
			c.Trace(fmt.Sprintf("Tag for reply is %v", t))
		}
		if t < 1 {
			panic(fmt.Sprintf("tag %d < 1", t))
		}
		if int(t-1) >= len(c.RPC) {
			panic(fmt.Sprintf("tag %d >= len(c.RPC) %d", t, len(c.RPC)))
		}
		c.Trace("RPC %v ", c.RPC[t-1])
		rrr := c.RPC[t-1]
		c.Trace("rrr %v ", rrr)
		rrr.Reply <- r.b
		c.Tags <- t
	}
}

func (c *Client) String() string {
	z := map[bool]string{false: "Alive", true: "Dead"}
	return fmt.Sprintf("%v tags available, Msize %v, %v FromNet %v ToNet %v", len(c.Tags), c.Msize, z[c.Dead],
		c.FromNet, c.ToNet)
}

func (s *Server) String() string {
	return fmt.Sprintf("Versioned %v Dead %v %d replies pending", s.Versioned, s.Dead, len(s.Replies))
}

func NewClient(opts ...ClientOpt) (*Client, error) {
	var c = &Client{}

	c.Tags = make(chan Tag, NumTags)
	for i := 1; i < int(NOTAG); i++ {
		c.Tags <- Tag(i)
	}
	c.FID = 1
	c.RPC = make([]*RPCCall, NumTags)
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, err
		}
	}
	c.FromClient = make(chan *RPCCall, NumTags)
	c.FromServer = make(chan *RPCReply)
	go c.IO()
	go c.readNetPackets()
	return c, nil
}

func (s *Server) beginSrvProfile() {
	var err error
	s.fprofile, err = ioutil.TempFile(filepath.Dir(*serverprofile), filepath.Base(*serverprofile))
	if err != nil {
		log.Fatal(err)
	}
	pprof.StartCPUProfile(s.fprofile)
}

func (s *Server) endSrvProfile() {
	pprof.StopCPUProfile()
	s.fprofile.Close()
	log.Println("writing cpuprofile to", s.fprofile.Name())
}

func (s *Server) readNetPackets() {
	if s.FromNet == nil {
		s.Dead = true
		return
	}
	defer s.FromNet.Close()
	defer s.ToNet.Close()
	if s.Trace != nil {
		s.Trace("Starting readNetPackets")
	}
	if *serverprofile != "" {
		s.beginSrvProfile()
		defer s.endSrvProfile()
	}
	for !s.Dead {
		l := make([]byte, 7)
		if n, err := s.FromNet.Read(l); err != nil || n < 7 {
			log.Printf("readNetPackets: short read: %v", err)
			s.Dead = true
			return
		}
		sz := int64(l[0]) + int64(l[1])<<8 + int64(l[2])<<16 + int64(l[3])<<24
		t := MType(l[4])
		b := bytes.NewBuffer(l[5:])
		r := io.LimitReader(s.FromNet, sz-7)
		if _, err := io.Copy(b, r); err != nil {
			log.Printf("readNetPackets: short read: %v", err)
			s.Dead = true
			return
		}
		if s.Trace != nil {
			s.Trace("readNetPackets: got %v, len %d, sending to IO", RPCNames[MType(l[4])], b.Len())
		}
		//panic(fmt.Sprintf("packet is %v", b.Bytes()[:]))
		//panic(fmt.Sprintf("s is %v", s))
		if err := s.D(s, b, t); err != nil {
			log.Printf("%v: %v", RPCNames[MType(l[4])], err)
		}
		if s.Trace != nil {
			s.Trace("readNetPackets: Write %v back", b)
		}
		amt, err := s.ToNet.Write(b.Bytes())
		if err != nil {
			log.Printf("readNetPackets: write error: %v", err)
			s.Dead = true
			return
		}
		if s.Trace != nil {
			s.Trace("Returned %v amt %v", b, amt)
		}
	}

}

func (s *Server) Start() {
	go s.readNetPackets()
}

func (s *Server) NineServer() NineServer {
	return s.NS
}

func NewServer(ns NineServer, opts ...ServerOpt) (*Server, error) {
	s := &Server{}
	s.Replies = make(chan RPCReply, NumTags)
	s.NS = ns
	s.D = Dispatch
	for _, o := range opts {
		if err := o(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Dispatch dispatches request to different functions.
// It's also the the first place we try to establish server semantics.
// We could do this with interface assertions and such a la rsc/fuse
// but most people I talked do disliked that. So we don't. If you want
// to make things optional, just define the ones you want to implement in this case.
func Dispatch(s *Server, b *bytes.Buffer, t MType) error {
	switch t {
	case Tversion:
		s.Versioned = true
	default:
		if !s.Versioned {
			m := fmt.Sprintf("Dispatch: %v not allowed before Tversion", RPCNames[t])
			// Yuck. Provide helper.
			d := b.Bytes()
			MarshalRerrorPkt(b, Tag(d[0])|Tag(d[1])<<8, m)
			return fmt.Errorf("Dispatch: %v not allowed before Tversion", RPCNames[t])
		}
	}
	switch t {
	case Tversion:
		return s.SrvRversion(b)
	case Tattach:
		return s.SrvRattach(b)
	case Tflush:
		return s.SrvRflush(b)
	case Twalk:
		return s.SrvRwalk(b)
	case Topen:
		return s.SrvRopen(b)
	case Tcreate:
		return s.SrvRcreate(b)
	case Tclunk:
		return s.SrvRclunk(b)
	case Tstat:
		return s.SrvRstat(b)
	case Twstat:
		return s.SrvRwstat(b)
	case Tremove:
		return s.SrvRremove(b)
	case Tread:
		return s.SrvRread(b)
	case Twrite:
		return s.SrvRwrite(b)
	}
	// This has been tested by removing Attach from the switch.
	ServerError(b, fmt.Sprintf("Dispatch: %v not supported", RPCNames[t]))
	return nil
}
