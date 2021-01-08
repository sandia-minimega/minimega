// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"fmt"
	"reflect"
	"testing"
)

func TestStringParse(t *testing.T) {
	vals := []fmt.Stringer{
		&KeyEvent{
			DownFlag: 0, Key: 0xff08,
		},
		&KeyEvent{
			DownFlag: 1, Key: 0x0030,
		},
		&KeyEvent{
			DownFlag: 1, Key: 0x0041,
		},
		&KeyEvent{
			DownFlag: 1, Key: 0xffe1,
		},
		&PointerEvent{
			ButtonMask: 0, XPosition: 100, YPosition: 200,
		},
		&PointerEvent{
			ButtonMask: 0, XPosition: 105, YPosition: 200,
		},
		&PointerEvent{
			ButtonMask: 1, XPosition: 105, YPosition: 205,
		},
	}

	for _, want := range vals {
		got, err := parseEvent(want.String())

		if err != nil {
			t.Errorf("parse failed -- %v", err)
			continue
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("struct aren't equal -- got: %v, want: %v", got, want)
		}
	}
}
