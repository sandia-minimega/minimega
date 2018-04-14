// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import "sync"

type Runner struct {
	tokens chan bool

	wg sync.WaitGroup

	once sync.Once // for error
	err  error
}

// NewRunner returns a runner that can be used to run things in parallel. A
// limit of 0 implies no limit in the number of parallel runs.
func NewRunner(limit int) *Runner {
	r := &Runner{}

	if limit < 1 {
		// no limit so make tokens return immediately
		r.tokens = make(chan bool)
		close(r.tokens)
	} else {
		r.tokens = make(chan bool, limit)

		// fill channel
		for i := 0; i < limit; i++ {
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

		if err := fn(); err != nil {
			r.once.Do(func() {
				r.err = err
			})
		}
	}()
}

// Error waits for all the functions to finish and returns the first error.
func (r *Runner) Error() error {
	r.wg.Wait()
	return r.err
}
