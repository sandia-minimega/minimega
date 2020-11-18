// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"testing"
	"time"
)

func TestCheckTimeLimit(t *testing.T) {
	c := Config{
		TimeLimit: 0,
	}

	if err := c.checkTimeLimit(100, 100*time.Minute); err != nil {
		t.Errorf("err != nil: %v", err)
	}

	c.TimeLimit = 10
	if err := c.checkTimeLimit(1, 10*time.Minute); err != nil {
		t.Errorf("err != nil: %v", err)
	}

	if err := c.checkTimeLimit(10, 10*time.Minute); err == nil {
		t.Errorf("err == nil: %v", err)
	}

	if err := c.checkTimeLimit(100, 10*time.Minute); err == nil {
		t.Errorf("err == nil: %v", err)
	}

	c.TimeLimit = 100
	if err := c.checkTimeLimit(10, 10*time.Minute); err != nil {
		t.Errorf("err != nil: %v", err)
	}
}

func TestSplitRange(t *testing.T) {
	c := Config{
		Prefix: "host",
		Start:  1,
		End:    4,
	}

	if v := c.splitRange("host[1-4]"); len(v) != 4 {
		t.Errorf("expected 4 not %v", v)
	}

	if v := c.splitRange("host[1-5]"); len(v) != 0 {
		t.Errorf("expected 0 not %v", v)
	}

	if v := c.splitRange("foo[1-4]"); len(v) != 0 {
		t.Errorf("expected 0 not %v", v)
	}
}
