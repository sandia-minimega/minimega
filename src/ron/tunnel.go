// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"fmt"
	log "minilog"
	"minitunnel"
	"net"
)

const (
	BUFFER_SIZE = 32768
)

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

func (s *Server) Reverse(filter *Client, source int, host string, dest int) error {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

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

func (c *Client) handleTunnel(server bool) {
	log.Debug("handleTunnel: %v", server)

	a, b := net.Pipe()

	c.tunnelData = make(chan []byte, 1024)

	go func() {
		if server {
			var err error
			c.tunnel, err = minitunnel.Dial(a)
			if err != nil {
				log.Errorln("Dial: %v", err)
				a.Close()
				b.Close()
			}
		} else {
			go func() {
				err := minitunnel.ListenAndServe(a)
				if err != nil {
					log.Fatalln("ListenAndServe: %v", err)
				}
			}()
		}
	}()

	go func() {
		for {
			var buf = make([]byte, BUFFER_SIZE)
			n, err := b.Read(buf)
			if err != nil {
				log.Errorln(err)
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

	for {
		data := <-c.tunnelData
		_, err := b.Write(data)
		if err != nil {
			log.Errorln(err)
			a.Close()
			b.Close()
			return
		}
	}
}
