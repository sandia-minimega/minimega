// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"errors"
	"fmt"
	log "minilog"
	"net"
)

// ListenUFS starts a listener to connect to UFS running on the VM specified by
// the UUID. Returns the TCP port or an error.
func (s *Server) ListenUFS(uuid string) (int, error) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c, ok := s.clients[uuid]
	if !ok {
		return 0, fmt.Errorf("no such client: %v", uuid)
	}

	m := &Message{
		Type:    MESSAGE_UFS,
		UUID:    uuid,
		UfsMode: UFS_OPEN,
	}
	if err := c.sendMessage(m); err != nil {
		return 0, err
	}

	// Listen on random tcp port
	l, err := net.Listen("tcp4", ":0")
	if err != nil {
		return 0, err
	}
	c.ufsListener = l

	addr := l.Addr().(*net.TCPAddr)

	go func() {
		defer l.Close()
		log.Info("waiting for connections to ufs on %v", addr)

		for {
			conn, err := l.Accept()
			if err != nil {
				log.Error("command socket: %v", err)
				continue
			}

			log.Info("new connection from %v", conn.RemoteAddr())
			c.ufsConn = conn

			Trunk(conn, c.UUID, func(m *Message) error {
				m.Type = MESSAGE_UFS
				m.UfsMode = UFS_DATA

				return c.sendMessage(m)
			})

			m := &Message{
				Type:    MESSAGE_UFS,
				UUID:    uuid,
				UfsMode: UFS_CLOSE,
			}
			if err := c.sendMessage(m); err != nil {
				log.Error("unable to close: %v", err)
			}
		}
	}()

	return addr.Port, nil
}

func (s *Server) DisconnectUFS(uuid string) error {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c, ok := s.clients[uuid]
	if !ok {
		return fmt.Errorf("no such client: %v", uuid)
	}

	if c.ufsListener == nil {
		return errors.New("ufs is not running")
	}

	m := &Message{
		Type:    MESSAGE_UFS,
		UUID:    uuid,
		UfsMode: UFS_CLOSE,
	}
	if err := c.sendMessage(m); err != nil {
		log.Error("unable to close: %v", err)
	}

	c.ufsListener.Close()

	if c.ufsConn != nil {
		c.ufsConn.Close()
	}

	return nil
}

func (c *client) ufsMessage(m *Message) {
	c.ufsConn.Write(m.Tunnel)
}
