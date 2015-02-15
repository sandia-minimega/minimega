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
