// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package meshage

import "sync"

// queue calls fn on each pushed message, one at a time and in push order,
// without blocking the caller. A goroutine runs only while messages are
// pending.
type queue struct {
	fn func(*Message)

	lock    sync.Mutex
	pending []*Message
	running bool
}

func newQueue(fn func(*Message)) *queue {
	return &queue{fn: fn}
}

func (q *queue) push(m *Message) {
	q.lock.Lock()
	defer q.lock.Unlock()

	q.pending = append(q.pending, m)

	if !q.running {
		q.running = true
		go q.drain()
	}
}

func (q *queue) drain() {
	for {
		q.lock.Lock()
		if len(q.pending) == 0 {
			// drop the array that a large backlog left behind
			q.pending = nil
			q.running = false
			q.lock.Unlock()
			return
		}

		m := q.pending[0]
		q.pending[0] = nil
		q.pending = q.pending[1:]
		q.lock.Unlock()

		q.fn(m)
	}
}
