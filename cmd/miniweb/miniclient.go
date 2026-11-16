// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"sync"

	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var mmMu sync.Mutex
var mm *miniclient.Conn

// noOp returns a closed channel
func noOp() chan *miniclient.Response {
	out := make(chan *miniclient.Response)
	close(out)
	return out
}

// drain discards the remaining responses so that miniclient.Conn.Run's
// goroutine can finish and release the connection lock after an early return.
func drain(ch chan *miniclient.Response) {
	go func() {
		for range ch {
		}
	}()
}

// run minimega commands, automatically redialing if we were disconnected
func run(c *Command) chan *miniclient.Response {
	mmMu.Lock()
	defer mmMu.Unlock()

	var err error

	if mm == nil {
		if mm, err = miniclient.Dial(*f_base); err != nil {
			log.Error("unable to dial: %v", err)
			return noOp()
		}
	}

	// any sticky error means the connection is unusable: close it and redial
	if mm.Error() != nil {
		mm.Close()
		if mm, err = miniclient.Dial(*f_base); err != nil {
			log.Error("unable to redial: %v", err)
			return noOp()
		}
	}

	log.Info("running: %v", c)
	return mm.Run(c.String())
}
