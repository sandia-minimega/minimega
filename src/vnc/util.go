// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package vnc

import (
	"encoding/binary"
	"fmt"
	"io"
)

type Writable interface {
	Write(io.Writer) error
}

func writeMessage(w io.Writer, msgType uint8, msg interface{}) error {
	if err := binary.Write(w, binary.BigEndian, &msgType); err != nil {
		return fmt.Errorf("unable to write message type -- %s", err.Error())
	}

	if err := binary.Write(w, binary.BigEndian, msg); err != nil {
		return fmt.Errorf("unable to write message -- %s", err.Error())
	}

	return nil
}
