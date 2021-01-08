// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"sync"
	"time"
)

type Runner struct {
	fn RunnerFn

	retries uint

	tokens chan bool

	wg sync.WaitGroup

	mu   sync.Mutex // guards below
	errs map[string]error
}

// DefaultRunner returns a runner with parameters based on igor.Config.
func DefaultRunner(fn RunnerFn) *Runner {
	r, err := NewRunner(fn, Limit(igor.ConcurrencyLimit), Retries(igor.CommandRetries))
	if err != nil {
		log.Fatal("invalid parameters: %v", err)
	}

	return r
}

type RunnerFn func(string) error

// NewRunner returns a runner that can be used to run fn in parallel.
func NewRunner(fn RunnerFn, options ...func(*Runner) error) (*Runner, error) {
	r := &Runner{
		fn:     fn,
		tokens: make(chan bool),
		errs:   make(map[string]error),
	}
	// assume no limit so make tokens return immediately
	close(r.tokens)

	for _, option := range options {
		if err := option(r); err != nil {
			return nil, err
		}
	}

	return r, nil
}

// Limit runner to v concurrent runs.
func Limit(v uint) func(*Runner) error {
	return func(r *Runner) error {
		if v > 0 {
			r.tokens = make(chan bool, v)

			// fill channel
			for i := uint(0); i < v; i++ {
				r.tokens <- true
			}
		}

		return nil
	}
}

// Retries specifies the number of times to rerun a function that returns an
// error.
func Retries(v uint) func(*Runner) error {
	return func(r *Runner) error {
		r.retries = v

		return nil
	}
}

// Run function on a host.
func (r *Runner) Run(host string) {
	r.wg.Add(1)

	go func() {
		defer r.wg.Done()

		// rate limit, only put token back if it was valid (closed channel
		// returns zero value)
		if v := <-r.tokens; v {
			defer func() {
				r.tokens <- v
			}()
		}

		// propagate error only when we run out of retries
		var err error

		for i := uint(0); i < r.retries+1; i++ {
			if i > 0 {
				time.Sleep(time.Second)
			}

			if err = r.fn(host); err == nil {
				break
			}

			log.Error("attempt %v/%v on %v, error: %v", i+1, r.retries+1, host, err)
		}

		if err != nil {
			r.mu.Lock()
			defer r.mu.Unlock()

			r.errs[host] = err
		}
	}()
}

// RunAll runs function on each host and returns r.Error().
func (r *Runner) RunAll(hosts []string) error {
	for _, host := range hosts {
		r.Run(host)
	}

	return r.Error()
}

// Error waits for all the functions to finish and returns an error if any had
// an error.
func (r *Runner) Error() error {
	r.wg.Wait()

	if len(r.errs) == 0 {
		return nil
	}

	hosts := []string{}
	for host, err := range r.errs {
		// too verbose?
		log.Error("host %v error: %v", host, err)
		hosts = append(hosts, host)
	}
	return fmt.Errorf("hosts with errors: %v", hosts)
}
