// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package vnc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

type Conn struct {
	net.Conn // embed

	// s tracks server info
	s Server
}

// Dial a VNC server and complete the handshake
func Dial(host string) (*Conn, error) {
	c, err := net.Dial("tcp", host)
	if err != nil {
		return nil, err
	}

	conn := &Conn{Conn: c}
	if err := conn.handshake(); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func (c *Conn) handshake() error {
	// Read protocol version
	buf := make([]byte, 12)
	if n, err := io.ReadFull(c, buf); err != nil {
		return fmt.Errorf("invalid server version: %v", string(buf[:n]))
	}

	// Respond with fixed version 3.3
	if _, err := io.WriteString(c, "RFB 003.003\n"); err != nil {
		return err
	}

	// Server sends a 4 byte security type
	buf = make([]byte, 4)
	n, err := c.Read(buf)
	if err != nil && n != 4 {
		return fmt.Errorf("invalid server security message: %v", string(buf[:n]))
	} else if err != nil && buf[3] != 0x01 { // the security type must be 1
		return fmt.Errorf("invalid server security type: %v", string(buf[:n]))
	}

	// Client sends an initialization message, non-zero here to indicate we will
	// allow a shared desktop.
	if _, err := c.Write([]byte{0x01}); err != nil {
		return err
	}

	// Read the server initialization
	if err = binary.Read(c, binary.BigEndian, &c.s.Width); err != nil {
		return errors.New("unable to read width")
	}
	if err = binary.Read(c, binary.BigEndian, &c.s.Height); err != nil {
		return errors.New("unable to read height")
	}
	if err = binary.Read(c, binary.BigEndian, &c.s.PixelFormat); err != nil {
		return errors.New("unable to read pixel format")
	}
	if err = binary.Read(c, binary.BigEndian, &c.s.NameLength); err != nil {
		return errors.New("unable to read name length")
	}

	c.s.Name = make([]uint8, c.s.NameLength)
	if err = binary.Read(c, binary.BigEndian, &c.s.Name); err != nil {
		return errors.New("unable to read name")
	}

	// Set Pixel format and encoding to something that we know how to decode.
	// We do this for all connections since both the framebuffer recorder and
	// the screenshot will try to read framebuffer updates.
	err = (&SetPixelFormat{
		PixelFormat: PixelFormat{
			BitsPerPixel: 32, Depth: 24, TrueColorFlag: 1,
			RedMax: 255, GreenMax: 255, BlueMax: 255,
			RedShift: 16, GreenShift: 8, BlueShift: 0,
		},
	}).Write(c.Conn)

	if err != nil {
		return fmt.Errorf("unable to set pixel format: %v", err)
	}

	err = (&SetEncodings{
		Encodings: []int32{RawEncoding, DesktopSizePseudoEncoding},
	}).Write(c.Conn)

	if err != nil {
		return fmt.Errorf("unable to set encodings: %v", err)
	}

	return nil
}
