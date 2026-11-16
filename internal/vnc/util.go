// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// writeMessage encodes the message type and the parts in big-endian order,
// and writes the message to w in one Write call.
func writeMessage(w io.Writer, msgType uint8, parts ...any) error {
	var buf bytes.Buffer
	buf.WriteByte(msgType)

	for _, part := range parts {
		if err := binary.Write(&buf, binary.BigEndian, part); err != nil {
			return fmt.Errorf("unable to encode message: %w", err)
		}
	}

	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("unable to write message: %w", err)
	}

	return nil
}
