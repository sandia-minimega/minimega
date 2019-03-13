package main

import (
	"testing"
	"time"
)

func TestIsActive(t *testing.T) {
	now := time.Now()

	r := &Reservation{
		Start: now.Add(-time.Hour),
		End:   now.Add(time.Hour),
	}

	if !r.IsActive(now) {
		t.Errorf("now should be active")
	}
	if r.IsActive(now.Add(2 * time.Hour)) {
		t.Errorf("now+2h should not be active")
	}
	if r.IsActive(now.Add(2 * time.Hour)) {
		t.Errorf("now-2h should not be active")
	}
}
