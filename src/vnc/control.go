// Copyright (2019) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package vnc

// VNC playback control
type Control int

const (
	Play Control = iota
	Pause
	Step
	LoadFile
)
