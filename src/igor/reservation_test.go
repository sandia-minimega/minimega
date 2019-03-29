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

func TestIsOverlap(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2006-01-02T15:00:00Z")

	r := &Reservation{
		Start: now.Add(-60 * time.Minute),
		End:   now.Add(60 * time.Minute),
	}

	// before/after reservation
	if r.IsOverlap(now.Add(-90*time.Minute), now.Add(-60*time.Minute)) {
		t.Errorf("[-90m, -60m] should not overlap")
	}
	if r.IsOverlap(now.Add(60*time.Minute), now.Add(90*time.Minute)) {
		t.Errorf("[60m, 90m] should not overlap")
	}

	// contains whole reservation
	if !r.IsOverlap(now.Add(-90*time.Minute), now.Add(90*time.Minute)) {
		t.Errorf("[-90m, 90m] should overlap")
	}

	// contained within reservation
	if !r.IsOverlap(now.Add(-30*time.Minute), now.Add(30*time.Minute)) {
		t.Errorf("[-30m, 30m] should overlap")
	}

	// overlap start/end
	if !r.IsOverlap(now.Add(-90*time.Minute), now.Add(-30*time.Minute)) {
		t.Errorf("[-90m, -30m] should overlap")
	}
	if !r.IsOverlap(now.Add(30*time.Minute), now.Add(90*time.Minute)) {
		t.Errorf("[30m, 90m] should overlap")
	}
}
