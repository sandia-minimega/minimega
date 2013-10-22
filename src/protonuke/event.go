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

// randomHost returns a host and the original specified text from the user
// command line. Therefore, if the user specified 10.0.0.0/24, randomHost may
// return (10.0.0.200, 10.0.0.0/24).
func randomHost() (host string, original string) {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	v := r.Intn(len(hosts))
	host = keys[v]
	original = hosts[host]
	return
}
