package main

import (
	"errors"
	"strings"
	"testing"
)

type stopTestCapture struct {
	err   error
	stops *int
}

func (c stopTestCapture) Type() string {
	return "pcap"
}

func (c stopTestCapture) Stop() error {
	(*c.stops)++

	return c.err
}

func TestCapturesStopContinuesAfterError(t *testing.T) {
	stopErr := errors.New("stop failed")
	var stops int
	captures := captures{
		m: map[int]capture{
			0: stopTestCapture{err: stopErr, stops: &stops},
			1: stopTestCapture{stops: &stops},
		},
	}

	err := captures.stop(func(capture) bool {
		return true
	})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected joined stop error, got %v", err)
	}
	if stops != 2 {
		t.Fatalf("expected both captures to be stopped, got %d stops", stops)
	}
	if len(captures.m) != 0 {
		t.Fatalf("expected all stopped captures to be removed, got %d entries", len(captures.m))
	}
}

func TestCaptureBridgeRefusesMissingBridge(t *testing.T) {
	const name = "mm_no_such_bridge"
	for _, f := range []func() error{
		func() error { return (&captures{}).CaptureBridge(name, "x.pcap") },
		func() error { return (&captures{}).CaptureNetflowFile(name, "x.nf") },
	} {
		err := f()
		if err == nil || !strings.Contains(err.Error(), `bridge "`+name+`" does not exist`) {
			t.Fatalf("expected missing bridge error, got %v", err)
		}
	}
	if _, ok := bridges.Lookup(name); ok {
		t.Fatal("bridge was created")
	}
}
