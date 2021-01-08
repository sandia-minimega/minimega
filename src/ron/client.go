// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"fmt"
	"io"
	log "minilog"
	"miniplumber"
	"minitunnel"
	"net"
	"strings"
	"sync"
	"time"
)

type Client struct {
	UUID     string
	Arch     string
	OS       string
	Version  string
	Hostname string
	IPs      []string
	MACs     []string

	// Processes that are running in the background
	Processes map[int]*Process

	// Tags set via the command socket since the last heartbeat. Also used by
	// the server to determine whether the client matches a given filter.
	Tags map[string]string

	// Responses for commands processed since the last heartbeat
	Responses []*Response

	// LastCommandID is the last command ID that the client processed.
	LastCommandID int
}

type client struct {
	*Client    // embed
	sync.Mutex // embed

	checkin time.Time // when the client last checked in

	tunnel *minitunnel.Tunnel

	// writeMu serializes calls to enc.Encode
	writeMu sync.Mutex

	conn io.ReadWriteCloser
	enc  *gob.Encoder
	dec  *gob.Decoder

	// maxCommandID is the highest command ID that we have processed for this
	// client. Should be reset if the command counter is reset.
	maxCommandID int

	// mangled is true if qemu flipped octets on us
	mangled bool

	// Namespace for the VM, set during handshake
	Namespace string

	// pipe readers and writers
	pipeLock    sync.Mutex
	pipeReaders map[string]*miniplumber.Reader
	pipeWriters map[string]chan<- string

	ufsListener net.Listener
	ufsConn     net.Conn
}

func (c *client) sendMessage(m *Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	return c.enc.Encode(m)
}

// Matches tests whether all the filters match the client.
func (c *Client) Matches(f *Filter) bool {
	if f == nil {
		return true
	}

	if f.UUID != "" && f.UUID != c.UUID {
		log.Debug("failed match on UUID: %v != %v", f.UUID, c.UUID)
		return false
	}
	if f.Hostname != "" && f.Hostname != c.Hostname {
		log.Debug("failed match on hostname: %v != %v", f.Hostname, c.Hostname)
		return false
	}
	if f.Arch != "" && f.Arch != c.Arch {
		log.Debug("failed match on arch: %v != %v", f.Arch, c.Arch)
		return false
	}
	if f.OS != "" && f.OS != c.OS {
		log.Debug("failed match on os: %v != %v", f.OS, c.OS)
		return false
	}

	for k, v := range f.Tags {
		if c.Tags[k] != v {
			log.Debug("failed match on tag %v, %v != %v", k, v, c.Tags[k])
			return false
		}
	}

	return c.matchesIP(f) && c.matchesMAC(f)
}

// matchesIP tests whether the IP or CIDR specified in the filter matches at
// least one of the client's IPs.
func (c *Client) matchesIP(f *Filter) bool {
	if f.IP == "" {
		return true
	}

	// special case, IPs can match on CIDRs as well as full IPs
	if strings.Contains(f.IP, "/") {
		_, ipnet, err := net.ParseCIDR(f.IP)
		if err != nil {
			log.Error("invalid CIDR %v: %v", f.IP, err)
			return false
		}

		for _, ip := range c.IPs {
			if ipnet.Contains(net.ParseIP(ip)) {
				return true
			}
			log.Debug("failed match on CIDR %v %v", f.IP, ip)
		}

		return false
	}

	i := net.ParseIP(f.IP)
	if i == nil {
		log.Error("invalid IP: %v", f.IP)
		return false
	}

	for _, ip := range c.IPs {
		if i.Equal(net.ParseIP(ip)) {
			return true
		}
		log.Debug("failed match on ip %v %v", f.IP, ip)
	}

	return false
}

// matchesMAC tests whether the MAC specified in the filter matches at least
// one of the client's MACs.
func (c *Client) matchesMAC(f *Filter) bool {
	if f.MAC == "" {
		return true
	}

	for _, mac := range c.MACs {
		if f.MAC == mac {
			return true
		}

		log.Debug("failed match on mac %v %v", f.MAC, mac)
	}

	return false
}

func (c *client) pipeHandler(plumber *miniplumber.Plumber, m *Message) {
	c.pipeLock.Lock()
	defer c.pipeLock.Unlock()

	pipe := m.Pipe
	if c.Namespace != "" {
		pipe = fmt.Sprintf("%v//%v", c.Namespace, m.Pipe)
	}

	switch m.PipeMode {
	case PIPE_NEW_READER:
		// register a new reader, if the client doesn't already have a
		// reader on this pipe
		if _, ok := c.pipeReaders[pipe]; !ok {
			p := plumber.NewReader(pipe)
			c.pipeReaders[pipe] = p
			go func() {
				defer func() {
					c.pipeLock.Lock()
					defer c.pipeLock.Unlock()
					delete(c.pipeReaders, pipe)
				}()
				for {
					select {
					case v := <-p.C:
						c.sendMessage(&Message{
							Type:     MESSAGE_PIPE,
							Pipe:     m.Pipe, // use the non-namespace pipe name for downstream
							PipeMode: PIPE_DATA,
							PipeData: v,
						})
					case <-p.Done:
						// signal the close downstream
						c.sendMessage(&Message{
							Type:     MESSAGE_PIPE,
							Pipe:     m.Pipe, // use the non-namespace pipe name for downstream
							PipeMode: PIPE_CLOSE_READER,
						})
						return
					}
				}
			}()
		}
	case PIPE_NEW_WRITER:
		if _, ok := c.pipeWriters[pipe]; !ok {
			p := plumber.NewWriter(pipe)
			c.pipeWriters[pipe] = p
		}
	case PIPE_CLOSE_READER:
		if p, ok := c.pipeReaders[pipe]; ok {
			// the reader goroutine will delete the reader from the
			// map. We do this because miniplumber can close the
			// reader for us asynchronously, and we want to clean
			// up accordingly.
			p.Close()
		}
	case PIPE_CLOSE_WRITER:
		if p, ok := c.pipeWriters[pipe]; ok {
			close(p)
			delete(c.pipeWriters, pipe)
		}
	case PIPE_DATA:
		// incoming data to the server is a write. The corresponding
		// data message in the miniccc client is a read.
		if p, ok := c.pipeWriters[pipe]; ok {
			p <- m.PipeData
		} else {
			log.Error("no such pipe: %v", pipe)
		}
	default:
		log.Error("unknown message type: %v", m.PipeMode)
		return
	}
}
