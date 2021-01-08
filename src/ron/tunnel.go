// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"fmt"
	"io"
	log "minilog"
)

const (
	BUFFER_SIZE = 32768
)

// Forward creates a tunnel from host->guest, based on UUID, source, host, and
// destination port. This is similar to the ssh -L command.
func (s *Server) Forward(uuid string, source int, host string, dest int) error {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c, ok := s.clients[uuid]
	if !ok {
		return fmt.Errorf("no such client: %v", uuid)
	}

	if c.tunnel == nil {
		return fmt.Errorf("tunnel has not been initialized for %v", uuid)
	}

	return c.tunnel.Forward(source, host, dest)
}

// Reverse creates a reverse tunnel from guest->host. It is possible to have
// multiple clients create a reverse tunnel simultaneously. filter allows
// specifying which clients to have create the tunnel.
func (s *Server) Reverse(filter *Filter, source int, host string, dest int) error {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	for _, c := range s.clients {
		if !c.Matches(filter) {
			continue
		}

		if c.tunnel == nil {
			return fmt.Errorf("tunnel has not been initialized for %v", c.UUID)
		}

		if err := c.tunnel.Reverse(source, host, dest); err != nil {
			return err
		}
	}

	return nil
}

// Trunk reads data from remote, constructs a *Message, and sends it using fn.
// Returns the first error.
func Trunk(remote io.ReadCloser, uuid string, fn func(*Message) error) {
	var n int
	var err error

	for err == nil {
		buf := make([]byte, 32*1024)
		n, err = remote.Read(buf)
		log.Debug("trunking %v bytes for %v", n, uuid)
		if err == nil {
			m := &Message{
				Type:   MESSAGE_TUNNEL,
				UUID:   uuid,
				Tunnel: buf[:n],
			}

			err = fn(m)
		}
	}

	if err != io.ErrClosedPipe && err != io.EOF {
		log.Error("trunk failed for %v: %v", uuid, err)
	}

	log.Debug("trunk exit for %v", uuid)
}
