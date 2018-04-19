// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

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

// DefaultRunner returns a runner with parameters based on igorConfig.
func DefaultRunner(fn RunnerFn) *Runner {
	return NewRunner(fn, igorConfig.ConcurrencyLimit, igorConfig.CommandRetries)
}

type RunnerFn func(string) error

// NewRunner returns a runner that can be used to run things in parallel. A
// limit of 0 implies no limit in the number of parallel runs. Retries
// specifies the number of times to rerun a function that returns an error. A
// value of 0 means only run the function once.
func NewRunner(fn RunnerFn, limit, retries uint) *Runner {
	r := &Runner{
		fn:      fn,
		retries: retries,
	}

	if limit < 1 {
		// no limit so make tokens return immediately
		r.tokens = make(chan bool)
		close(r.tokens)
	} else {
		r.tokens = make(chan bool, limit)

		// fill channel
		for i := uint(0); i < limit; i++ {
			r.tokens <- true
		}
	}

	return r
}

// Run another function.
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
			if err = r.fn(host); err == nil {
				break
			}

			log.Error("attempt %v/%v on %v, error: %v", i+1, r.retries+1, host, err)
			time.Sleep(time.Second)
		}

		if err != nil {
			r.mu.Lock()
			defer r.mu.Unlock()

			r.errs[host] = err
		}
	}()
}

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
