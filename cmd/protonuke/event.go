// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"math/rand"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type EventTicker struct {
	mean   time.Duration
	stddev time.Duration
	min    time.Duration
	max    time.Duration
	count  uint64
}

func NewEventTicker(mean, stddev, min, max time.Duration) *EventTicker {
	return &EventTicker{
		mean:   mean,
		stddev: stddev,
		min:    min,
		max:    max,
	}
}

func Milliseconds(t time.Duration) float64 {
	msec := t.Nanoseconds() / int64(time.Millisecond)
	nsec := t.Nanoseconds() % int64(time.Millisecond)
	return float64(msec) + float64(nsec)*(1e-9*1000)
}

func (e *EventTicker) Tick() {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	t := time.Duration(int64(r.NormFloat64()*Milliseconds(e.stddev)+Milliseconds(e.mean)) * int64(time.Millisecond))

	// truncate to min and max
	if t < e.min {
		t = e.min
	} else if t > e.max {
		t = e.max
	}

	log.Debug("tick time %v", t)

	time.Sleep(time.Duration(t))
}

// randomHost returns a host and the original specified text from the user
// command line. Therefore, if the user specified 10.0.0.0/24, randomHost may
// return (10.0.0.200, 10.0.0.0/24).
func randomHost() (host string, original string) {
	if len(hosts) == 0 || hosts == nil {
		return "", ""
	}
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	v := r.Intn(len(hosts))
	host = keys[v]
	original = hosts[host]
	return
}
