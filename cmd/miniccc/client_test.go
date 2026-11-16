// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/gob"
	"io"
	"sync"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

func TestResolveClientUUIDUsesOverride(t *testing.T) {
	discovered := false
	got := resolveClientUUID(" A5BA6920-5BCF-4022-B8CF-015425F7B05C ", func() string {
		discovered = true
		return "unexpected"
	})
	if discovered {
		t.Fatal("UUID discovery ran despite an explicit override")
	}
	if want := "a5ba6920-5bcf-4022-b8cf-015425f7b05c"; got != want {
		t.Fatalf("unexpected UUID: got %q, want %q", got, want)
	}
}

func TestResolveClientUUIDDiscoversDefault(t *testing.T) {
	const want = "3b440429-067f-5b75-a0c3-f436519f8ccb"
	got := resolveClientUUID("", func() string { return want })
	if got != want {
		t.Fatalf("unexpected UUID: got %q, want %q", got, want)
	}
}

// While the link is down (enc == nil) background senders must get an error,
// not crash the agent.
func TestSendMessageWhileDisconnected(t *testing.T) {
	setEncoder(nil)

	if err := sendMessage(&ron.Message{Type: ron.MESSAGE_PIPE}); err == nil {
		t.Fatal("sendMessage succeeded with no connection")
	}
}

// Reconnect swaps the encoder while senders are active; must not race.
func TestSendMessageDuringReconnect(t *testing.T) {
	defer setEncoder(nil)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				sendMessage(&ron.Message{Type: ron.MESSAGE_PIPE})
			}
		}()
	}

	for j := 0; j < 200; j++ {
		setEncoder(gob.NewEncoder(io.Discard))
		setEncoder(nil)
	}
	wg.Wait()
}
