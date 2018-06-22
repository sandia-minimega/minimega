// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import "testing"

func TestCheckTimeLimit(t *testing.T) {
	old := igorConfig.TimeLimit
	defer func() {
		igorConfig.TimeLimit = old
	}()

	igorConfig.TimeLimit = 0
	if err := checkTimeLimit(100, 100); err != nil {
		t.Errorf("err != nil: %v", err)
	}

	igorConfig.TimeLimit = 10
	if err := checkTimeLimit(1, 10); err != nil {
		t.Errorf("err != nil: %v", err)
	}

	igorConfig.TimeLimit = 100
	if err := checkTimeLimit(10, 10); err != nil {
		t.Errorf("err != nil: %v", err)
	}

	igorConfig.TimeLimit = 10
	if err := checkTimeLimit(10, 10); err == nil {
		t.Errorf("err == nil: %v", err)
	}

	igorConfig.TimeLimit = 10
	if err := checkTimeLimit(100, 10); err == nil {
		t.Errorf("err == nil: %v", err)
	}
}
