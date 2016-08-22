// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"minicli"
	"strconv"
	"sync"
	"time"
)

// vmRate is the average rate to launch VMs
var vmRate = time.Second / 4

// vmBurst is the maximum burst size
var vmBurst = 10

var vmLimiter = NewLimiter(vmRate, vmBurst)

// Limiter implements a token bucket
type Limiter struct {
	sync.Mutex // embed

	tokens int
	last   time.Time

	rate  time.Duration
	burst int
}

var vmLimiterCLIHanders = []minicli.Handler{
	{ // vm limit
		HelpShort: "configure VM launch rate limiting",
		HelpLong: `
The "vm limit" API controls the rate at which VMs are launched. If VMs are
launched too quickly, it can cause problems for systems such as KSM. minimega
implements rate limiting using a token bucket with two parameters:

- rate: average time between launching VMs
- burst: maximum burst size when VMs have not been launched for some time

See https://en.wikipedia.org/wiki/Token_bucket for more info.
		`,
		Patterns: []string{
			"vm limit <rate,> [rate]",
			"vm limit <burst,> [burst]",
		},
		Call: wrapBroadcastCLI(cliVMLimit),
	},
}

// NewLimiter create a token bucket of size burst and replenished at rate rate.
func NewLimiter(rate time.Duration, burst int) *Limiter {
	return &Limiter{
		last:  time.Now(),
		rate:  rate,
		burst: burst,
	}
}

// SetRate updates the amortized rate between Waits
func (l *Limiter) SetRate(rate time.Duration) {
	l.Lock()
	defer l.Unlock()

	l.update()
	l.rate = rate
}

// Rate returns the current rate
func (l *Limiter) Rate() time.Duration {
	l.Lock()
	defer l.Unlock()

	return l.rate
}

// SetBurst updates the burst limit
func (l *Limiter) SetBurst(burst int) {
	l.Lock()
	defer l.Unlock()

	l.update()
	l.burst = burst
}

// Burst returns the current burst limit
func (l *Limiter) Burst() int {
	l.Lock()
	defer l.Unlock()

	return l.burst
}

// Wait for a token to be available
func (l *Limiter) Wait() {
	l.Lock()
	defer l.Unlock()

	for {
		l.update()

		if l.tokens > 0 {
			l.tokens--
			return
		}

		// Compute time until next token becomes available
		time.Sleep(l.rate - time.Since(l.last))

		// Token *should* be available
	}
}

func (l *Limiter) update() {
	// Compute how many tokens should be available since the last time that we
	// called update
	n := int(time.Since(l.last) / l.rate)
	l.tokens += n

	if l.tokens > l.burst {
		l.tokens = l.burst
	}

	// There's some error caused by rounding so only advance by n * rate
	l.last = l.last.Add(time.Duration(n) * l.rate)
}

func cliVMLimit(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["rate"] {
		rate := c.StringArgs["rate"]
		if rate == "" {
			resp.Response = vmLimiter.Rate().String()
			return nil
		}

		r, err := time.ParseDuration(rate)
		if err != nil {
			return err
		}

		if r <= 0 {
			return errors.New("rate > 0")
		}

		vmLimiter.SetRate(r)
	} else if c.BoolArgs["burst"] {
		burst := c.StringArgs["burst"]
		if burst == "" {
			resp.Response = strconv.Itoa(vmLimiter.Burst())
			return nil
		}

		b, err := strconv.Atoi(burst)
		if err != nil {
			return err
		}

		if b < 1 {
			return errors.New("burst limit >= 1")
		}

		vmLimiter.SetBurst(b)
	} else {
		return errors.New("unreachable")
	}

	return nil
}
