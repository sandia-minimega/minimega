// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package vnc

// See RFC 6143 Section 7.3.2
type Server struct {
	Width  uint16
	Height uint16
	PixelFormat
	NameLength uint32
	Name       []uint8
}
