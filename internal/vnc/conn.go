// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// clientFormat is the pixel format that the client sends after the
// handshake. The decoder knows how to decode it.
var clientFormat = PixelFormat{
	BitsPerPixel:  32,
	Depth:         24,
	TrueColorFlag: 1,
	RedMax:        255,
	GreenMax:      255,
	BlueMax:       255,
	RedShift:      16,
	GreenShift:    8,
	BlueShift:     0,
}

// handshakeTimeout is the time that Dial has to connect and to complete the
// handshake. Callers can hold locks of the namespace during Dial.
const handshakeTimeout = 10 * time.Second

type Conn struct {
	net.Conn // embed

	// s tracks server info. The reader goroutine changes Width and Height
	// on a DesktopSize rectangle; use size to read them.
	s Server

	// pf is the pixel format that the client sent. The decoder uses it.
	pf PixelFormat

	mu sync.Mutex // guards s.Width and s.Height after the handshake
}

// Dial a VNC server and complete the handshake. Dial gives an error if the
// connection and the handshake take longer than handshakeTimeout.
func Dial(host string) (*Conn, error) {
	return dial(host, handshakeTimeout)
}

func dial(host string, timeout time.Duration) (*Conn, error) {
	c, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return nil, err
	}

	return newConn(c, timeout)
}

// newConn completes the handshake on c in timeout. It closes c if the
// handshake fails.
func newConn(c net.Conn, timeout time.Duration) (*Conn, error) {
	conn := &Conn{Conn: c}

	err := c.SetDeadline(time.Now().Add(timeout))
	if err == nil {
		err = conn.handshake()
	}
	if err == nil {
		err = c.SetDeadline(time.Time{})
	}
	if err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func (c *Conn) handshake() error {
	// Read protocol version
	buf := make([]byte, 12)
	if n, err := io.ReadFull(c, buf); err != nil {
		return fmt.Errorf("invalid server version %q: %w", buf[:n], err)
	}

	// Respond with fixed version 3.3
	if _, err := io.WriteString(c, "RFB 003.003\n"); err != nil {
		return err
	}

	// Server sends a 4 byte security type
	var security uint32
	if err := binary.Read(c, binary.BigEndian, &security); err != nil {
		return fmt.Errorf("unable to read server security type: %v", err)
	}

	switch security {
	case securityNone:
	case 0:
		var n uint32
		if err := binary.Read(c, binary.BigEndian, &n); err != nil {
			return fmt.Errorf("server refused the connection: %v", err)
		}
		reason := make([]byte, min(n, 1024))
		if _, err := io.ReadFull(c, reason); err != nil {
			return fmt.Errorf("server refused the connection: %v", err)
		}
		return fmt.Errorf("server refused the connection: %q", reason)
	default:
		return fmt.Errorf("server requires security type %d, only None (1) is supported", security)
	}

	// Client sends an initialization message, non-zero here to indicate we will
	// allow a shared desktop.
	if _, err := c.Write([]byte{0x01}); err != nil {
		return err
	}

	// Read the server initialization
	if err := binary.Read(c, binary.BigEndian, &c.s.Width); err != nil {
		return errors.New("unable to read width")
	}
	if err := binary.Read(c, binary.BigEndian, &c.s.Height); err != nil {
		return errors.New("unable to read height")
	}
	if err := binary.Read(c, binary.BigEndian, &c.s.PixelFormat); err != nil {
		return errors.New("unable to read pixel format")
	}
	if err := binary.Read(c, binary.BigEndian, &c.s.NameLength); err != nil {
		return errors.New("unable to read name length")
	}

	if c.s.NameLength > maxCutText {
		return fmt.Errorf("server name has %d bytes, limit is %d", c.s.NameLength, maxCutText)
	}
	c.s.Name = make([]uint8, c.s.NameLength)
	if err := binary.Read(c, binary.BigEndian, &c.s.Name); err != nil {
		return errors.New("unable to read name")
	}

	// Set Pixel format and encoding to something that we know how to decode.
	// We do this for all connections since both the framebuffer recorder and
	// the screenshot will try to read framebuffer updates.
	if err := (&SetPixelFormat{PixelFormat: clientFormat}).Write(c.Conn); err != nil {
		return fmt.Errorf("unable to set pixel format: %v", err)
	}
	c.pf = clientFormat

	err := (&SetEncodings{
		Encodings: []int32{RawEncoding, DesktopSizePseudoEncoding},
	}).Write(c.Conn)

	if err != nil {
		return fmt.Errorf("unable to set encodings: %v", err)
	}

	return nil
}

// size returns the framebuffer size.
func (c *Conn) size() (uint16, uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.s.Width, c.s.Height
}

func (c *Conn) setSize(w, h uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.s.Width, c.s.Height = w, h
}
