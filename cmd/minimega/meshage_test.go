// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"sync"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/iomeshage"
	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestMeshageClassify(t *testing.T) {
	table := func(rows, cols int) meshageResponse {
		var r meshageResponse
		for i := 0; i < rows; i++ {
			r.Tabular = append(r.Tabular, make([]string, cols))
		}
		return r
	}

	for _, tt := range []struct {
		name string
		body interface{}
		want meshage.Class
	}{
		{"command", meshageCommand{Command: minicli.Command{Original: "vm info"}}, meshage.Control},
		{"small response", table(10, 10), meshage.Control},
		{"large response", table(1000, 11), meshage.Bulk},
		{"VM launch", meshageVMLaunch{}, meshage.Bulk},
		{"VM response", meshageVMResponse{}, meshage.Control},
		{"file", iomeshage.Message{}, meshage.Bulk},
		{"log", meshageLogMessage{}, meshage.Log},
		{"status", meshageStatusMessage{}, meshage.Control},
	} {
		if got := meshageClassify(tt.body); got != tt.want {
			t.Errorf("%v: class = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestMeshageLogWriterDrop checks that a full queue drops log lines and
// counts them, and that a stopped writer queues nothing.
func TestMeshageLogWriterDrop(t *testing.T) {
	w := &meshageLogWriter{
		dest:  "head",
		queue: make(chan meshageLogMessage, 1),
		stop:  make(chan struct{}),
	}

	line := []byte("2026/11/06 19:05:01 INFO vm.go:1: launching\n")
	w.log(line)
	w.log(line)
	w.log(line)

	if got := len(w.queue); got != 1 {
		t.Errorf("queued %d lines, want 1", got)
	}
	if got := w.dropped.Load(); got != 2 {
		t.Errorf("dropped %d lines, want 2", got)
	}

	<-w.queue
	close(w.stop)
	w.log(line)
	if got := len(w.queue); got != 0 {
		t.Errorf("stopped writer queued %d lines, want 0", got)
	}
}

// TestStopMeshageLoggingConcurrent stops mesh logging from more than one
// goroutine at the same time. Each writer must stop one time only.
func TestStopMeshageLoggingConcurrent(t *testing.T) {
	for round := 0; round < 10; round++ {
		w := &meshageLogWriter{
			dest:  "head",
			queue: make(chan meshageLogMessage, 1),
			stop:  make(chan struct{}),
		}

		meshLogLock.Lock()
		meshLogWriter = w
		logMeshNode = "head"
		meshLogLock.Unlock()

		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				stopMeshageLogging()
				getLogMeshNode()
			}()
		}
		wg.Wait()

		select {
		case <-w.stop:
		default:
			t.Fatal("the writer did not stop")
		}
		if node := getLogMeshNode(); node != "" {
			t.Errorf("log mesh node = %q after stop, want empty", node)
		}

		meshLogLock.Lock()
		if meshLogWriter != nil {
			t.Error("writer is set after stop")
		}
		meshLogLock.Unlock()
	}
}
