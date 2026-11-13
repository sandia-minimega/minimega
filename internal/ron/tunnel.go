// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/minitunnel"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	BUFFER_SIZE = 32768
)

// ErrNoClients is the error that Reverse wraps when no client matches the
// filter.
var ErrNoClients = errors.New("no clients")

// errReversePending is the error for a client whose previous reverse request
// has not finished.
var errReversePending = errors.New("a reverse request is already pending")

// reverseTimeout is how long Reverse waits for each client to open its
// listener.
var reverseTimeout = 30 * time.Second

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

func (s *Server) ListForwards(uuid string) (map[int]string, error) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c, ok := s.clients[uuid]
	if !ok {
		return nil, fmt.Errorf("no such client: %v", uuid)
	}

	if c.tunnel == nil {
		return nil, fmt.Errorf("tunnel has not been initialized for %v", uuid)
	}

	return c.tunnel.ListForwards(), nil
}

func (s *Server) CloseForward(uuid string, id int) error {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c, ok := s.clients[uuid]
	if !ok {
		return fmt.Errorf("no such client: %v", uuid)
	}

	if c.tunnel == nil {
		return fmt.Errorf("tunnel has not been initialized for %v", uuid)
	}

	return c.tunnel.CloseForward(id)
}

// ListReverses returns the reverse tunnels that the client with the given UUID
// opened and that are still open. A client without an initialized tunnel has
// no reverse tunnels.
func (s *Server) ListReverses(uuid string) ([]minitunnel.ReverseTunnel, error) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c, ok := s.clients[uuid]
	if !ok {
		return nil, fmt.Errorf("no such client: %v", uuid)
	}

	if c.tunnel == nil {
		return nil, nil
	}

	return c.tunnel.ListReverses(), nil
}

// Reverse creates a reverse tunnel from guest->host. It is possible to have
// multiple clients create a reverse tunnel simultaneously. filter allows
// specifying which clients to have create the tunnel, and matches clients the
// same way as commands do. Reverse returns an error when no client matches
// filter. Otherwise, it tries every matching client and returns one error that
// names each client that failed. A client whose previous reverse request has
// not finished is skipped and named in the error.
func (s *Server) Reverse(filter *Filter, source int, host string, dest int) error {
	type pending struct {
		c      *client
		tunnel *minitunnel.Tunnel
	}

	failed := map[string]error{}
	tunnels := map[string]pending{}

	s.clientLock.Lock()
	for uuid, c := range s.clients {
		if s.UseVMs && !s.loadVMTags(uuid, c, []*Filter{filter}) {
			continue
		}

		if !c.Matches(filter) {
			continue
		}

		if c.tunnel == nil {
			failed[uuid] = errors.New("tunnel has not been initialized")
			continue
		}

		if c.reversePending {
			failed[uuid] = errReversePending
			continue
		}

		c.reversePending = true
		tunnels[uuid] = pending{c, c.tunnel}
	}
	s.clientLock.Unlock()

	if len(failed)+len(tunnels) == 0 {
		if f := filter.String(); f != "" {
			return fmt.Errorf("%w match filter: %v", ErrNoClients, f)
		}

		return fmt.Errorf("%w connected", ErrNoClients)
	}

	type result struct {
		uuid string
		err  error
	}

	results := make(chan result, len(tunnels))
	for uuid, p := range tunnels {
		done := func() {
			s.clientLock.Lock()
			defer s.clientLock.Unlock()

			p.c.reversePending = false
		}

		go func() {
			results <- result{uuid, reverse(p.tunnel, source, host, dest, reverseTimeout, done)}
		}()
	}

	for range tunnels {
		if r := <-results; r.err != nil {
			failed[r.uuid] = r.err
		}
	}

	uuids := make([]string, 0, len(failed))
	for uuid := range failed {
		uuids = append(uuids, uuid)
	}
	sort.Strings(uuids)

	var errs []error
	for _, uuid := range uuids {
		errs = append(errs, fmt.Errorf("%v: %w", uuid, failed[uuid]))
	}

	return errors.Join(errs...)
}

// reverse calls tunnel.Reverse and stops waiting after timeout, so a client
// that never answers cannot block the caller. The client can still open the
// listener after the timeout. reverse calls finished when tunnel.Reverse
// returns, which can be after reverse returns.
func reverse(tunnel *minitunnel.Tunnel, source int, host string, dest int, timeout time.Duration, finished func()) error {
	done := make(chan error, 1)
	go func() {
		err := tunnel.Reverse(source, host, dest)
		finished()
		done <- err
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("no response from client after %v", timeout)
	}
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
