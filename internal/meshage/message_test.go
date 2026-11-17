// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package meshage

import (
	"encoding/gob"
	"net"
	"testing"
	"time"
)

const orderCount = 2000

func newTestNode(t *testing.T, name string) *Node {
	t.Helper()

	n := newNode(name, "test", 0, net.IPv4bcast, 0, "test")
	go n.messageHandler()

	return n
}

// TestMessageHandlerDeliversInOrder checks that messages for this node reach
// the receive channel in the order they arrived.
func TestMessageHandlerDeliversInOrder(t *testing.T) {
	n := newTestNode(t, "b")

	go func() {
		for i := 0; i < orderCount; i++ {
			n.messagePump <- &Message{
				Recipients: []string{"b"},
				Source:     "a",
				Command:    MESSAGE,
				Body:       i,
			}
		}
	}()

	for want := 0; want < orderCount; want++ {
		select {
		case m := <-n.receive:
			if got := m.Body.(int); got != want {
				t.Fatalf("received message %d, want %d", got, want)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for message %d", want)
		}
	}
}

// TestMessageHandlerDoesNotBlock checks that a full receive channel does not
// stop the handler from accepting more messages.
func TestMessageHandlerDoesNotBlock(t *testing.T) {
	n := newTestNode(t, "b")

	total := 2*RECEIVE_BUFFER + 10
	sent := make(chan struct{})
	go func() {
		for i := 0; i < total; i++ {
			n.messagePump <- &Message{
				Recipients: []string{"b"},
				Source:     "a",
				Command:    MESSAGE,
				Body:       i,
			}
		}
		close(sent)
	}()

	select {
	case <-sent:
	case <-time.After(10 * time.Second):
		t.Fatal("messageHandler blocked on a full receive channel")
	}

	for want := 0; want < total; want++ {
		if got := (<-n.receive).Body.(int); got != want {
			t.Fatalf("received message %d, want %d", got, want)
		}
	}
}

// addTestClient connects n to a fake client named name over net.Pipe and
// returns the messages that client receives. The client ACKs each message
// unless ack is false.
func addTestClient(t *testing.T, n *Node, name string, ack bool) <-chan *Message {
	t.Helper()

	local, remote := net.Pipe()
	t.Cleanup(func() {
		local.Close()
		remote.Close()
	})

	c := &client{
		name: name,
		conn: local,
		enc:  gob.NewEncoder(local),
		dec:  gob.NewDecoder(local),
		ack:  make(chan uint64, RECEIVE_BUFFER),
	}

	n.clientLock.Lock()
	n.clients[name] = c
	n.clientLock.Unlock()

	n.meshLock.Lock()
	n.routes[name] = name
	n.meshLock.Unlock()

	// read ACKs from the client, as clientHandler would
	go func() {
		for {
			var m Message
			if err := c.dec.Decode(&m); err != nil {
				return
			}
			c.ack <- m.ID
		}
	}()

	received := make(chan *Message, 4*orderCount)
	go func() {
		enc := gob.NewEncoder(remote)
		dec := gob.NewDecoder(remote)
		for {
			var m Message
			if err := dec.Decode(&m); err != nil {
				return
			}
			if ack {
				if err := enc.Encode(Message{Command: ACK, ID: m.ID}); err != nil {
					return
				}
			}
			received <- &m
		}
	}()

	return received
}

// TestMessageHandlerRelaysInOrder checks that messages relayed to another node
// keep their order for each source, and that local copies are still delivered.
func TestMessageHandlerRelaysInOrder(t *testing.T) {
	n := newTestNode(t, "b")
	received := addTestClient(t, n, "c", true)

	// interleave two sources, each also addressed to b itself
	go func() {
		for i := 0; i < orderCount; i++ {
			for _, src := range []string{"a1", "a2"} {
				n.messagePump <- &Message{
					Recipients: []string{"b", "c"},
					Source:     src,
					Command:    MESSAGE,
					Body:       i,
				}
			}
		}
	}()

	for i := 0; i < 2*orderCount; i++ {
		select {
		case <-n.receive:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for local message %d", i)
		}
	}

	next := map[string]int{}
	for i := 0; i < 2*orderCount; i++ {
		select {
		case m := <-received:
			if got := m.Body.(int); got != next[m.Source] {
				t.Fatalf("relayed message %d from %v, want %d", got, m.Source, next[m.Source])
			}
			next[m.Source]++
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for relayed message %d", i)
		}
	}
}

// TestRelayDoesNotWaitOnOtherHops checks that a next hop that never ACKs does
// not hold up messages from the same source to another hop.
func TestRelayDoesNotWaitOnOtherHops(t *testing.T) {
	n := newTestNode(t, "b")
	n.timeout = time.Minute

	healthy := addTestClient(t, n, "c", true)
	stuck := addTestClient(t, n, "d", false)

	n.messagePump <- &Message{Recipients: []string{"d"}, Source: "a", Command: MESSAGE, Body: -1}
	for i := 0; i < 100; i++ {
		n.messagePump <- &Message{Recipients: []string{"c"}, Source: "a", Command: MESSAGE, Body: i}
	}

	select {
	case <-stuck:
	case <-time.After(10 * time.Second):
		t.Fatal("message to d was not relayed")
	}

	for want := 0; want < 100; want++ {
		select {
		case m := <-healthy:
			if got := m.Body.(int); got != want {
				t.Fatalf("relayed message %d to c, want %d", got, want)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("message %d to c waited on the unacknowledged message to d", want)
		}
	}
}

// TestQueueRunsInOrder checks that a queue calls its function in push order
// and stops its goroutine when empty.
func TestQueueRunsInOrder(t *testing.T) {
	var got []int
	done := make(chan struct{}, orderCount)
	q := newQueue(func(m *Message) {
		got = append(got, m.Body.(int))
		done <- struct{}{}
	})

	for round := 0; round < 2; round++ {
		for i := 0; i < orderCount/2; i++ {
			q.push(&Message{Body: round*orderCount/2 + i})
		}
		for i := 0; i < orderCount/2; i++ {
			<-done
		}

		// let the drain goroutine notice the queue is empty
		deadline := time.Now().Add(10 * time.Second)
		for {
			q.lock.Lock()
			running := q.running
			q.lock.Unlock()
			if !running {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("queue goroutine did not stop when empty")
			}
			time.Sleep(time.Millisecond)
		}
	}

	for want, v := range got {
		if v != want {
			t.Fatalf("queue ran message %d at position %d", v, want)
		}
	}
}

// TestQueueReleasesBacklog checks that an empty queue does not keep the array
// of a drained backlog.
func TestQueueReleasesBacklog(t *testing.T) {
	release := make(chan struct{})
	q := newQueue(func(m *Message) {
		<-release
	})

	// the first message blocks the drain, so the others accumulate
	for i := 0; i < 101; i++ {
		q.push(&Message{Body: i})
	}
	close(release)

	deadline := time.Now().Add(10 * time.Second)
	for {
		q.lock.Lock()
		running, held := q.running, cap(q.pending)
		q.lock.Unlock()
		if !running {
			if held != 0 {
				t.Fatalf("empty queue keeps an array for %d messages", held)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("queue goroutine did not stop when empty")
		}
		time.Sleep(time.Millisecond)
	}
}
