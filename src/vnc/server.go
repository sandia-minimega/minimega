// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package vnc

// See RFC 6143 Section 7.3.2
type Server struct {
	Width  uint16
	Height uint16
	PixelFormat
	NameLength uint32
	Name       []uint8
}
