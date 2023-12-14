// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"encoding/binary"
	"fmt"
	"io"
)

func writeMessage(w io.Writer, msgType uint8, msg interface{}) error {
	if err := binary.Write(w, binary.BigEndian, &msgType); err != nil {
		return fmt.Errorf("unable to write message type -- %s", err.Error())
	}

	if err := binary.Write(w, binary.BigEndian, msg); err != nil {
		return fmt.Errorf("unable to write message -- %s", err.Error())
	}

	return nil
}
