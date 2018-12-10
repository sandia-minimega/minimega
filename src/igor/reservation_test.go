package main

import (
	"testing"
	"time"
)

func TestActive(t *testing.T) {
	now := time.Now()

	r := Reservation{
		StartTime: now.Add(-time.Hour).Unix(),
		EndTime:   now.Add(time.Hour).Unix(),
	}

	if !r.Active(now) {
		t.Errorf("now should be active")
	}
	if r.Active(now.Add(2 * time.Hour)) {
		t.Errorf("now+2h should not be active")
	}
	if r.Active(now.Add(2 * time.Hour)) {
		t.Errorf("now-2h should not be active")
	}
}
