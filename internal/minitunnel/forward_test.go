// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minitunnel_test

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	. "github.com/sandia-minimega/minimega/v2/internal/minitunnel"
)

// Closing forwards while other forwards register and clients connect must not
// race on the forward table or a forward's connections.
func TestCloseForwardConcurrent(t *testing.T) {
	g, h := pipe(t)
	goListenAndServe(g)

	tun, err := Dial(h)
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	ports := make([]int, n)
	for i := range ports {
		ports[i] = freePort(t)
	}

	// forward 1 is the one we close while the rest register
	if err := tun.Forward(ports[0], "127.0.0.1", 1); err != nil {
		t.Fatal(err)
	}
	for len(tun.ListForwards()) < 1 {
		time.Sleep(time.Millisecond)
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { // register more forwards
		defer wg.Done()
		for _, p := range ports[1:] {
			tun.Forward(p, "127.0.0.1", 1)
		}
	}()
	go func() { // connect clients to forward 1
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ports[0])); err == nil {
				c.Close()
			}
		}
	}()
	go func() { // close forwards, ids start at 1
		defer wg.Done()
		for id := 1; id <= n; id++ {
			tun.CloseForward(id)
		}
	}()
	wg.Wait()
}
