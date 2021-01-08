// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

type Counter struct {
	vals chan int
	done chan bool
}

// NewCounter creates a channel of IDs and a goroutine to populate the channel
// with a counter. This is useful for assigning UIDs to fields since the
// goroutine will (almost) never repeat the same value (unless we hit IntMax).
func NewCounter() *Counter {
	res := Counter{
		vals: make(chan int),
		done: make(chan bool),
	}

	go func() {
		defer close(res.vals)

		for i := 0; ; i++ {
			select {
			case res.vals <- i:
			case <-res.done:
				return
			}
		}
	}()

	return &res
}

// Next gets the next value from the counter. Calling Next after Stop will
// cause this to return the zero value.
func (c *Counter) Next() int {
	return <-c.vals
}

// Stop terminates a counter. Should only be called once otherwise it will
// cause a panic.
func (c *Counter) Stop() {
	close(c.done)
}
