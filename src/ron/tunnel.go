// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"fmt"
	"io"
	log "minilog"
	"minitunnel"
	"net"
)

const (
	BUFFER_SIZE = 32768
)

// Create a tunnel from host->guest, based on UUID, source, host, and
// destination port. This is similar to the ssh -L command.
func (s *Server) Forward(uuid string, source int, host string, dest int) error {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if c, ok := s.clients[uuid]; ok {
		if c.tunnel == nil {
			return fmt.Errorf("tunnel object is nil!")
		}
		return c.tunnel.Forward(source, host, dest)
	} else {
		return fmt.Errorf("no such client: %v", uuid)
	}
}

// Create a reverse tunnel from guest->host. It is possible to have multiple
// clients create a reverse tunnel simultaneously. filter allows specifying
// which clients to have create the tunnel.
func (s *Server) Reverse(filter *Client, source int, host string, dest int) error {
	// TODO: can we safely do this without holding s.clientLock?

	command := &Command{
		Filter: filter,
	}

	for _, c := range s.clients {
		if c.matchFilter(command) {
			if c.tunnel == nil {
				return fmt.Errorf("tunnel object is nil on client: %v", c.UUID)
			}
			err := c.tunnel.Reverse(source, host, dest)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// tunnel transport handler
func (c *Client) handleTunnel(server bool, stop chan bool) {
	log.Debug("handleTunnel: %v", server)

	a, b := net.Pipe()

	c.tunnelData = make(chan []byte, 1024)

	go func() {
		if server {
			var err error
			c.tunnel, err = minitunnel.Dial(a)
			if err != nil {
				log.Error("Dial: %v", err)
				a.Close()
				b.Close()
			}
		} else {
			go func() {
				err := minitunnel.ListenAndServe(a)
				if err != nil {
					log.Fatal("ListenAndServe: %v", err)
				}
			}()
		}
	}()

	go func() {
		for {
			var buf = make([]byte, BUFFER_SIZE)
			n, err := b.Read(buf)
			if err != nil {
				if err != io.ErrClosedPipe {
					log.Errorln(err)
				}
				a.Close()
				b.Close()
				return
			}

			// push it up in a message
			m := &Message{
				Type:   MESSAGE_TUNNEL,
				UUID:   c.UUID,
				Tunnel: buf[:n],
			}

			c.out <- m
		}
	}()

	go func() {
		for {
			data := <-c.tunnelData
			if data == nil {
				return
			}
			_, err := b.Write(data)
			if err != nil {
				log.Errorln(err)
				a.Close()
				b.Close()
				return
			}
		}
	}()

	<-stop
	log.Debug("ron client tunnel close: %v", c.UUID)
	a.Close()
	b.Close()
	close(c.tunnelData)
}
