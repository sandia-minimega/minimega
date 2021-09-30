// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

type Type int

// Message types to inform the mux on either end how to route the message
const (
	MESSAGE_COMMAND Type = iota
	MESSAGE_CLIENT
	MESSAGE_TUNNEL
	MESSAGE_FILE
	MESSAGE_PIPE
	MESSAGE_UFS
	MESSAGE_HEARTBEAT
)

// Pipe modes
const (
	PIPE_NEW_READER = iota
	PIPE_NEW_WRITER
	PIPE_CLOSE_READER
	PIPE_CLOSE_WRITER
	PIPE_DATA
)

// UFS modes
const (
	UFS_OPEN = iota
	UFS_CLOSE
	UFS_DATA
)

type Message struct {
	Type  Type
	UUID  string
	Error string

	// MESSAGE_COMMAND
	Commands map[int]*Command

	// MESSAGE_CLIENT
	Client *Client

	// MESSAGE_FILE
	File *File

	// MESSAGE_TUNNEL and MESSAGE_UFS
	Tunnel []byte

	// MESSAGE_PIPE
	Pipe     string
	PipeMode int
	PipeData string

	// MESSAGE_UFS
	UfsMode int
}

func (t Type) String() string {
	switch t {
	case MESSAGE_COMMAND:
		return "COMMAND"
	case MESSAGE_CLIENT:
		return "CLIENT"
	case MESSAGE_TUNNEL:
		return "TUNNEL"
	case MESSAGE_FILE:
		return "FILE"
	case MESSAGE_PIPE:
		return "PIPE"
	case MESSAGE_UFS:
		return "UFS"
	case MESSAGE_HEARTBEAT:
		return "HEARTBEAT"
	}

	return "UNKNOWN"
}
