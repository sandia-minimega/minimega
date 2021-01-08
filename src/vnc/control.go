// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package vnc

// VNC playback control
type Control int

const (
	Play Control = iota
	Pause
	Step
	LoadFile
	WaitForIt
	ClickIt
)
