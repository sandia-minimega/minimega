// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"fmt"
	log "minilog"
	"net"
	"os"
)

func (s *Server) Mount(uuid string, dst string) error {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c, ok := s.clients[uuid]
	if !ok {
		return fmt.Errorf("no such client: %v", uuid)
	}

	if err := os.Mkdir(dst, 0700); err != nil {
		return err
	}

	m := &Message{
		Type:    MESSAGE_UFS,
		UUID:    uuid,
		UfsMode: UFS_OPEN,
	}
	if err := c.sendMessage(m); err != nil {
		return err
	}

	// Is it possible to do this without creating a domain socket? It looks
	// like maybe we could pass file descriptors to 9p.
	l, err := net.Listen("unix", dst+"-unix")
	if err != nil {
		return err
	}

	go func() {
		log.Info("waiting for connections to ufs")
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Error("command socket: %v", err)
				continue
			}

			c.rootFS.conn = conn

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

	return nil
	//syscall.Mount(dst+"-unix", dst, "9p", 0, "trans=unix,noextend")
}

func (s *Server) Unmount(uuid string) error {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c, ok := s.clients[uuid]
	if !ok {
		return fmt.Errorf("no such client: %v", uuid)
	}

	// TODO
	/*
		syscall.Umount
	*/

	m := &Message{
		Type:    MESSAGE_UFS,
		UUID:    uuid,
		UfsMode: UFS_CLOSE,
	}
	if err := c.sendMessage(m); err != nil {
		log.Error("unable to close: %v", err)
	}

	return nil
}

func (c *client) ufsMessage(m *Message) {
	c.rootFS.conn.Write(m.Tunnel)
}
