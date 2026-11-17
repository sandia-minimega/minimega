package minitunnel

import (
	"testing"
	"time"
)

// A mux send blocked on a full queue must give up when the TID is removed,
// otherwise one stalled stream blocks every other stream on the transport.
func TestMuxSendReleasedByRemove(t *testing.T) {
	c := makeChans()
	in := c.add(1)
	for len(in) < cap(in) {
		in <- &tunnelMessage{}
	}

	sent := make(chan bool)
	go func() { sent <- c.send(1, &tunnelMessage{}) }()

	select {
	case <-sent:
		t.Fatal("send to a full queue returned before remove")
	case <-time.After(50 * time.Millisecond):
	}

	c.remove(1)

	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Fatal("send still blocked after remove")
	}

	if c.send(1, &tunnelMessage{}) {
		t.Fatal("send to a removed TID reported success")
	}
}
