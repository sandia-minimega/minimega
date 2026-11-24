// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestFCInstanceStartLimit starts more VMs than the limit. The starts take
// longer in total than the API timeout, but each request is shorter, so all
// of them must succeed.
func TestFCInstanceStartLimit(t *testing.T) {
	oldParallel, oldLaunch, oldTimeout := *f_firecrackerStartParallel, *f_firecrackerLaunchParallel, fcStartTimeout
	t.Cleanup(func() {
		*f_firecrackerStartParallel = oldParallel
		*f_firecrackerLaunchParallel = oldLaunch
		fcStartTimeout = oldTimeout
		fcStartSem = fcSemaphore{size: fcStartParallel}
	})
	*f_firecrackerStartParallel = 2
	// the launch limit does not apply to starts
	*f_firecrackerLaunchParallel = 5
	fcStartSem = fcSemaphore{size: fcStartParallel}

	const (
		starts  = 10
		work    = 150 * time.Millisecond
		timeout = 500 * time.Millisecond
	)

	var mu sync.Mutex
	running, most := 0, 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/actions" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		running++
		most = max(most, running)
		mu.Unlock()

		time.Sleep(work)

		mu.Lock()
		running--
		mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	})

	// A short directory keeps the socket path under the size limit.
	dir, err := os.MkdirTemp("", "fcs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	sock := filepath.Join(dir, "api.sock")
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = fcListen(t, sock)
	srv.Start()
	t.Cleanup(srv.Close)

	fcStartTimeout = timeout

	errs := make(chan error, starts)
	begin := time.Now()
	for range starts {
		go func() {
			errs <- fcInstanceStart(newFCClient(sock))
		}()
	}
	for range starts {
		if err := <-errs; err != nil {
			t.Errorf("fcInstanceStart() = %v", err)
		}
	}

	if most != 2 {
		t.Errorf("at most %d InstanceStart requests ran at the same time, want 2", most)
	}
	if d := time.Since(begin); d <= timeout {
		t.Errorf("the starts took %v; the test needs more than the timeout %v", d, timeout)
	}
}

// TestFCInstanceStartTimeout checks that InstanceStart has its own timeout,
// and not the timeout of the other API requests.
func TestFCInstanceStartTimeout(t *testing.T) {
	oldTimeout := fcStartTimeout
	t.Cleanup(func() {
		fcStartTimeout = oldTimeout
		fcStartSem = fcSemaphore{size: fcStartParallel}
	})
	fcStartSem = fcSemaphore{size: fcStartParallel}

	const work = 300 * time.Millisecond

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(work):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusNoContent)
	})

	dir, err := os.MkdirTemp("", "fcs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	sock := filepath.Join(dir, "api.sock")
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = fcListen(t, sock)
	srv.Start()
	t.Cleanup(srv.Close)

	c := newFCClient(sock)
	c.timeout = work / 3

	fcStartTimeout = 3 * work
	if err := fcInstanceStart(c); err != nil {
		t.Errorf("start longer than the API timeout = %v, want no error", err)
	}

	fcStartTimeout = work / 3
	if err := fcInstanceStart(c); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("start longer than the start timeout = %v, want deadline exceeded", err)
	}
}
