// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"sort"
	"strconv"
	"testing"
	"time"
)

func init() {
	log.LevelFlag = log.INFO
	log.VerboseFlag = true

	log.Init()
}

func TestNextFree(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2006-01-02T15:00:00Z")

	reservations := []*Reservation{
		makeReservation(now, "-10m", "15m"),
		makeReservation(now, "10m", "15m"),
		makeReservation(now, "30m", "15m"),
	}

	first := reservations[0].Start

	// should be able to schedule between 0 and 1
	res := nextFree(reservations, first, 5*time.Minute)
	if res != reservations[0].End {
		t.Errorf("expected %v not %v", reservations[0].End, res)
	}

	// should only be able to schedule after 2
	res = nextFree(reservations, first, 15*time.Minute)
	if res != reservations[2].End {
		t.Errorf("expected %v not %v", reservations[2].End, res)
	}

	// should be able to schedule between 1 and 2
	res = nextFree(reservations, reservations[1].Start, 5*time.Minute)
	if res != reservations[1].End {
		t.Errorf("expected %v not %v", reservations[1].End, res)
	}
}

func TestScheduleHosts(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2006-01-02T15:00:00Z")

	r := &Reservations{
		M: map[uint64]*Reservation{
			0: makeReservation(now, "-10m", "15m"),
			1: makeReservation(now, "10m", "15m"),
			2: makeReservation(now, "30m", "15m"),
		},
	}
	igor.Config.Padlen = 1
	igor.Config.Prefix = "host"
	igor.Config.Start = 1
	igor.Config.End = 4

	res := makeReservation(now, "", "5m")
	mustSchedule(t, r, res, false)
	t.Logf("%v", res)

	res = makeReservation(now, "", "15m")
	mustSchedule(t, r, res, false)
	t.Logf("%v", res)
}

func TestScheduleContiguous(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2006-01-02T15:00:00Z")

	r := &Reservations{
		M: map[uint64]*Reservation{},
	}

	igor.Config.Padlen = 1
	igor.Config.Prefix = "host"
	igor.Config.Start = 1
	igor.Config.End = 4

	// schedule a bunch of 1, 2, 3, 4 node reservations which should fill an
	// hour almost entirely, leaving two slots open.
	for i := 0; i < 19; i++ {
		log.Info("reservation #%v", i)
		res := makeReservation(now, "0m", "5m")
		res.Hosts = make([]string, i%4+1)
		mustSchedule(t, r, res, true)

		t.Logf("res #%v scheduled from %v to %v on hosts %v", i, res.Start, res.End, res.Hosts)
	}

	// should be two open slots (one for 45-50 and one for 55-60)
	for i := 0; i < 2; i++ {
		res := makeReservation(now, "0m", "5m")
		res.Hosts = make([]string, 1)
		mustSchedule(t, r, res, true)

		if res.End.Sub(now) > time.Hour {
			t.Errorf("should have been within the hour")
		}
		t.Logf("res #%v scheduled from %v to %v on hosts %v", i, res.Start, res.End, res.Hosts)
	}
}

func TestScheduleContiguousGap(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2006-01-02T15:00:00Z")

	igor.Config.Padlen = 1
	igor.Config.Prefix = "host"
	igor.Config.Start = 1
	igor.Config.End = 4

	for i := igor.Config.Start; i < igor.Config.End; i++ {
		for j := i + 1; j < igor.Config.End+1; j++ {
			r := &Reservations{
				M: map[uint64]*Reservation{},
			}

			// create reservation on host i, j
			res := makeReservation(now, "0m", "5m")
			res.Hosts = []string{"host" + strconv.Itoa(i), "host" + strconv.Itoa(j)}
			sort.Strings(res.Hosts)
			mustSchedule(t, r, res, true)

			// Make sure that we schedule after
			res2 := makeReservation(now, "0m", "5m")
			res2.Hosts = make([]string, (igor.Config.End - igor.Config.Start))
			log.Info("reserving n hosts: %v", len(res2.Hosts))
			mustSchedule(t, r, res2, false)

			if res2.Start != res.End {
				t.Errorf("scheduling error: res2 should start at %v not %v", res.End, res2.Start)
			}
		}
	}
}

func TestScheduleContiguousFragmented(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2006-01-02T15:00:00Z")

	igor.Config.Padlen = 1
	igor.Config.Prefix = "host"
	igor.Config.Start = 1
	igor.Config.End = 4

	r := &Reservations{
		M: map[uint64]*Reservation{},
	}

	// create reservation on host 1, 4 for 5 minutes from now
	res := makeReservation(now, "5m", "5m")
	res.Hosts = []string{"host1", "host4"}
	mustSchedule(t, r, res, true)

	// try to schedule 4 nodes for 5 minutes (should fit before)
	res2 := makeReservation(now, "0m", "5m")
	res2.Hosts = make([]string, 4)
	mustSchedule(t, r, res2, false)

	log.Info("res2 start time: %v", res2.Start)
	if res2.Start != now {
		t.Errorf("res2 should start now")
	}

	// try to schedule 4 nodes for 10 minutes (should go after)
	res3 := makeReservation(now, "0m", "10m")
	res3.Hosts = make([]string, 4)
	mustSchedule(t, r, res3, false)

	log.Info("res3 start time: %v", res3.Start)
	if res3.Start != res.End {
		t.Errorf("res3 should after res")
	}

	// add another 5 minute reservation
	res4 := makeReservation(now, "15m", "5m")
	res4.Hosts = []string{"host1", "host4"}
	mustSchedule(t, r, res4, true)

	// try to schedule res3 again (should go after res4 now)
	res3 = makeReservation(now, "0m", "10m")
	res3.Hosts = make([]string, 4)
	mustSchedule(t, r, res3, false)

	log.Info("res3 start time: %v", res3.Start)
	if res3.Start != res4.End {
		t.Errorf("res3 should after res")
	}
}

func benchmarkScheduleContiguous(width int, b *testing.B) {
	now, _ := time.Parse(time.RFC3339, "2006-01-02T15:00:00Z")

	r := &Reservations{
		M: map[uint64]*Reservation{},
	}

	igor.Config.Padlen = 1
	igor.Config.Prefix = "host"
	igor.Config.Start = 1
	igor.Config.End = width

	for i := 0; i < b.N; i++ {
		res := makeReservation(now, "0m", "5m")
		res.Hosts = make([]string, i%width+1)

		if err := scheduleContiguous(r, res); err != nil {
			b.Errorf("unable to schedule: %v", err)
		}

		r.M[uint64(len(r.M))] = res
	}
}

func BenchmarkScheduleContiguous4(b *testing.B)   { benchmarkScheduleContiguous(4, b) }
func BenchmarkScheduleContiguous16(b *testing.B)  { benchmarkScheduleContiguous(16, b) }
func BenchmarkScheduleContiguous64(b *testing.B)  { benchmarkScheduleContiguous(64, b) }
func BenchmarkScheduleContiguous256(b *testing.B) { benchmarkScheduleContiguous(256, b) }

func makeReservation(now time.Time, start, duration string) *Reservation {
	d, _ := time.ParseDuration(duration)

	r := &Reservation{
		Duration: d,
		Hosts:    []string{"host1", "host2", "host3", "host4"},
	}

	if start != "" {
		d, _ := time.ParseDuration(start)
		r.Start = now.Add(d)
		r.End = r.Start.Add(r.Duration)
	}

	return r
}

func mustSchedule(t *testing.T, r *Reservations, res *Reservation, keep bool) {
	var err error
	if res.Hosts[0] == "" {
		err = scheduleContiguous(r, res)
	} else {
		err = scheduleHosts(r, res)
	}

	log.Info("scheduled from %v to %v: %v", res.Start, res.End, res.Hosts)

	if err != nil {
		t.Errorf("unable to schedule: %v", err)
	}

	if keep {
		r.M[uint64(len(r.M))] = res
	}
}
