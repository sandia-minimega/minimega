// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// -suggest runs cliSetup and minicli.Suggest without ever starting meshage, so
// hostname completion must not panic when meshageNode is nil.
func TestSuggestHostnameWithoutMesh(t *testing.T) {
	orig := meshageNode
	meshageNode = nil
	defer func() { meshageNode = orig }()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("minicli.Suggest panicked: %v", r)
		}
	}()

	if got := minicli.Suggest("mesh send "); len(got) != 0 {
		t.Fatalf("expected no suggestions without a mesh, got %v", got)
	}
}

// A SIGINT during teardown (after Shutdown closed the channel) must not crash
// the process with a send on a closed channel.
func TestSignalAfterShutdownDoesNotPanic(t *testing.T) {
	origShutdown := shutdown
	shutdown = make(chan os.Signal, 1)
	defer func() { shutdown = origShutdown; shutdownMu = sync.Mutex{} }()

	// keep SIGINT handled so the test process survives once main's channel
	// is unregistered
	other := make(chan os.Signal, 1)
	signal.Notify(other, os.Interrupt)
	defer signal.Stop(other)

	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Shutdown blocks forever after closing the channel
	go Shutdown("test")
	<-shutdown

	// the "user" presses Ctrl-C while teardown is running
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}

	select {
	case <-other:
	case <-time.After(2 * time.Second):
		t.Fatal("SIGINT not delivered")
	}
	time.Sleep(100 * time.Millisecond)
}
