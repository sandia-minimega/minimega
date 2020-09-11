// Copyright 2012 The Ninep Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// This code is imported from the old ninep repo,
// with some changes.

package protocol

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const DefaultAddr = ":5640"

// Server is a 9p server.
// For now it's extremely serial. But we will use a chan for replies to ensure that
// we can go to a more concurrent one later.
type Server struct {
	NS NineServer
	D  Dispatcher

	// TCP address to listen on, default is DefaultAddr
	Addr string

	// trace function for logging
	trace Tracer

	// mu guards below
	mu sync.Mutex

	listeners map[net.Listener]struct{}
}

type conn struct {
	// server on which the connection arrived.
	server *Server

	// rwc is the underlying network connection.
	rwc net.Conn

	// remoteAddr is rwc.RemoteAddr().String(). See note in net/http/server.go.
	remoteAddr string

	// replies
	replies chan RPCReply

	// dead is set to true when we finish reading packets.
	dead bool
}

func NewServer(ns NineServer, opts ...ServerOpt) (*Server, error) {
	s := &Server{
		NS:    ns,
		D:     Dispatch,
		trace: nologf,
	}

	for _, o := range opts {
		if err := o(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func Trace(tracer Tracer) ServerOpt {
	return func(s *Server) error {
		if tracer == nil {
			return errors.New("tracer cannot be nil")
		}
		s.trace = tracer
		return nil
	}
}

// nologf does nothing and is the default trace function
func nologf(format string, args ...interface{}) {}

func (s *Server) newConn(rwc net.Conn) *conn {
	c := &conn{
		server:  s,
		rwc:     rwc,
		replies: make(chan RPCReply, NumTags),
	}

	return c
}

// trackListener from http.Server
func (s *Server) trackListener(ln net.Listener, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listeners == nil {
		s.listeners = make(map[net.Listener]struct{})
	}

	if add {
		s.listeners[ln] = struct{}{}
	} else {
		delete(s.listeners, ln)
	}
}

// closeListenersLocked from http.Server
func (s *Server) closeListenersLocked() error {
	var err error
	for ln := range s.listeners {
		if cerr := ln.Close(); cerr != nil && err == nil {
			err = cerr
		}
		delete(s.listeners, ln)
	}
	return err
}

// ListenAndServe starts a new Listener on e.Addr and then calls serve.
func (s *Server) ListenAndServe() error {
	addr := s.Addr
	if addr == "" {
		addr = DefaultAddr
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return s.Serve(ln)
}

// Serve accepts incoming connections on the Listener and calls e.Accept on
// each connection.
func (s *Server) Serve(ln net.Listener) error {
	defer ln.Close()

	var tempDelay time.Duration // how long to sleep on accept failure

	s.trackListener(ln, true)
	defer s.trackListener(ln, false)

	// from http.Server.Serve
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				s.trace("ufs: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return err
		}
		tempDelay = 0

		if err := s.Accept(conn); err != nil {
			return err
		}
	}
}

// Accept a new connection, typically called via Serve but may be called
// directly if there's a connection from an exotic listener.
func (s *Server) Accept(conn net.Conn) error {
	c := s.newConn(conn)

	go c.serve()
	return nil
}

// Shutdown closes all active listeners. It does not close all active
// connections but probably should.
func (s *Server) Shutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.closeListenersLocked()
}

func (s *Server) String() string {
	// TODO
	return ""
}

func (c *conn) String() string {
	return fmt.Sprintf("Dead %v %d replies pending", c.dead, len(c.replies))
}

func (c *conn) logf(format string, args ...interface{}) {
	// prepend some info about the conn
	c.server.trace("[%v] "+format, append([]interface{}{c.remoteAddr}, args...)...)
}

func (c *conn) serve() {
	if c.rwc == nil {
		c.dead = true
		return
	}

	c.remoteAddr = c.rwc.RemoteAddr().String()

	defer c.rwc.Close()

	c.logf("Starting readNetPackets")

	for !c.dead {
		l := make([]byte, 7)
		if n, err := c.rwc.Read(l); err != nil || n < 7 {
			c.logf("readNetPackets: short read: %v", err)
			c.dead = true
			return
		}
		sz := int64(l[0]) + int64(l[1])<<8 + int64(l[2])<<16 + int64(l[3])<<24
		t := MType(l[4])
		b := bytes.NewBuffer(l[5:])
		r := io.LimitReader(c.rwc, sz-7)
		if _, err := io.Copy(b, r); err != nil {
			c.logf("readNetPackets: short read: %v", err)
			c.dead = true
			return
		}
		c.logf("readNetPackets: got %v, len %d, sending to IO", RPCNames[MType(l[4])], b.Len())
		//panic(fmt.Sprintf("packet is %v", b.Bytes()[:]))
		//panic(fmt.Sprintf("s is %v", s))
		if err := c.server.D(c.server, b, t); err != nil {
			c.logf("%v: %v", RPCNames[MType(l[4])], err)
		}
		c.logf("readNetPackets: Write %v back", b)
		amt, err := c.rwc.Write(b.Bytes())
		if err != nil {
			c.logf("readNetPackets: write error: %v", err)
			c.dead = true
			return
		}
		c.logf("Returned %v amt %v", b, amt)
	}
}

func (s *Server) NineServer() NineServer {
	return s.NS
}

// Dispatch dispatches request to different functions.
// It's also the the first place we try to establish server semantics.
// We could do this with interface assertions and such a la rsc/fuse
// but most people I talked do disliked that. So we don't. If you want
// to make things optional, just define the ones you want to implement in this case.
func Dispatch(s *Server, b *bytes.Buffer, t MType) error {
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
