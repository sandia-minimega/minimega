// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package iomeshage

import (
	"os"
	"strconv"
)

type MessageType int

const (
	TYPE_INFO MessageType = iota
	TYPE_WHOHAS
	TYPE_XFER
	TYPE_RESPONSE
)

// Message is the only structure sent between iomeshage nodes (including ACKS).
// It is used as the body of a meshage message.
type Message struct {
	// Type of message
	Type MessageType

	From     string
	Filename string
	Perm     os.FileMode
	Glob     []string
	Part     int64
	TID      int64
	ACK      bool
	Data     []byte
}

func (m MessageType) String() string {
	switch m {
	case TYPE_INFO:
		return "INFO"
	case TYPE_WHOHAS:
		return "WHOHAS"
	case TYPE_XFER:
		return "XFER"
	case TYPE_RESPONSE:
		return "RESPONSE"
	}

	return "MessageType(" + strconv.Itoa(int(m)) + ")"
}
