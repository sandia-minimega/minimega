// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "minilog"
	"sync"
	"time"
)

type Runner struct {
	retries uint

	tokens chan bool

	wg sync.WaitGroup

	once sync.Once // for error
	err  error
}

// DefaultRunner returns a runner with parameters based on igorConfig.
func DefaultRunner() *Runner {
	return NewRunner(igorConfig.ConcurrencyLimit, igorConfig.CommandRetries)
}

// NewRunner returns a runner that can be used to run things in parallel. A
// limit of 0 implies no limit in the number of parallel runs. Retries
// specifies the number of times to rerun a function that returns an error. A
// value of 0 means only run the function once.
func NewRunner(limit, retries uint) *Runner {
	r := &Runner{
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
func (r *Runner) Run(fn func() error) {
	r.wg.Add(1)

	go func() {
		defer r.wg.Done()

		// rate limit, only put token back if it was valid (closed channel
		// returns zero value)
		if v := <-r.tokens; v {
			r.tokens <- v
		}

		for i := uint(0); i < r.retries+1; i++ {
			if err := fn(); err != nil {
				r.once.Do(func() {
					r.err = err
				})

				log.Error("attempt %v/%v, error: %v", i+1, r.retries+1, err)
				time.Sleep(time.Second)
				continue
			}

			break
		}
	}()
}

// Error waits for all the functions to finish and returns the first error.
func (r *Runner) Error() error {
	r.wg.Wait()
	return r.err
}
