package main

import (
	"math/rand"
	log "minilog"
	"time"
)

type EventTicker struct {
	mean   int
	stddev int
	min    int
	max    int
	count  uint64
}

func NewEventTicker(mean, stddev, min, max int) *EventTicker {
	return &EventTicker{
		mean:   mean,
		stddev: stddev,
		min:    min,
		max:    max,
	}
}

func (e *EventTicker) Tick() {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	t := int(r.NormFloat64()*float64(e.stddev) + float64(e.mean))

	// truncate to min and max
	if t < e.min {
		t = e.min
	} else if t > e.max {
		t = e.max
	}

	log.Debug("tick time %vms", t)

	time.Sleep(time.Duration(t) * time.Millisecond)
}
