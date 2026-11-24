// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"
	"sync"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// OVSBatching sends the OVS port adds of netns taps through the batcher. KVM
// and container taps always add their ports one at a time.
var OVSBatching = true

const (
	ovsBatchMax      = 256
	ovsBatchArgBytes = 100 << 10
)

type ovsPortReq struct {
	bridge string
	tap    string
	vlan   int
	host   bool
	res    chan error
}

func (r *ovsPortReq) args() []string {
	args := []string{"--", "add-port", r.bridge, r.tap}
	if r.vlan != 0 {
		args = append(args, fmt.Sprintf("tag=%v", r.vlan))
	}
	if r.host {
		args = append(args, "--", "set", "Interface", r.tap, "type=internal")
	}
	return args
}

// ovsBatcher commits queued port adds in groups: one ovs-vsctl transaction
// per group of at most max requests. A request does not wait for others. The
// requests that arrive while a transaction runs make the next group.
type ovsBatcher struct {
	run func([]string) (string, error)
	max int

	once  sync.Once
	queue chan *ovsPortReq
}

func newOVSBatcher(run func([]string) (string, error), max int) *ovsBatcher {
	return &ovsBatcher{run: run, max: max}
}

var ovsPorts = newOVSBatcher(func(args []string) (string, error) {
	return ovsCmdWrapper(args)
}, ovsBatchMax)

// ovsAddPortBatched is ovsAddPort through the process-wide batcher.
func ovsAddPortBatched(bridge, tap string, vlan int, host bool) error {
	return ovsPorts.add(bridge, tap, vlan, host)
}

func (o *ovsBatcher) add(bridge, tap string, vlan int, host bool) error {
	o.once.Do(func() {
		o.queue = make(chan *ovsPortReq, o.max)
		go o.loop()
	})

	// see note in ovsAddBridge.
	if len(tap) > 15 {
		log.Warn("tap name is longer than 15 characters.. dragons ahead")
	}

	r := &ovsPortReq{bridge: bridge, tap: tap, vlan: vlan, host: host, res: make(chan error, 1)}
	o.queue <- r
	return <-r.res
}

func (o *ovsBatcher) loop() {
	for r := range o.queue {
		o.flush(o.collect(r))
	}
}

// collect returns r and the requests that are in the queue now, at most max
// in total. It does not wait for more requests.
func (o *ovsBatcher) collect(r *ovsPortReq) []*ovsPortReq {
	batch := []*ovsPortReq{r}

	for len(batch) < o.max {
		select {
		case r := <-o.queue:
			batch = append(batch, r)
		default:
			return batch
		}
	}

	return batch
}

// flush rejects repeated tap names, then commits the batch in chunks that
// keep the argument list under ovsBatchArgBytes.
func (o *ovsBatcher) flush(batch []*ovsPortReq) {
	seen := map[string]bool{}
	var todo []*ovsPortReq

	for _, r := range batch {
		if seen[r.tap] {
			r.res <- fmt.Errorf("add port failed: %v %v in the same batch", r.tap, errAlreadyExists)
			continue
		}
		seen[r.tap] = true
		todo = append(todo, r)
	}

	for len(todo) > 0 {
		n, size := 0, 0
		for n < len(todo) {
			s := 0
			for _, a := range todo[n].args() {
				s += len(a) + 1
			}
			if n > 0 && size+s > ovsBatchArgBytes {
				break
			}
			size += s
			n++
		}

		o.commit(todo[:n])
		todo = todo[n:]
	}
}

// commit runs the batch as one transaction. A transaction is atomic, so on
// failure it splits the batch in half and retries each half.
func (o *ovsBatcher) commit(batch []*ovsPortReq) {
	var args []string
	for _, r := range batch {
		args = append(args, r.args()...)
	}

	_, err := o.run(args)
	if err == nil {
		for _, r := range batch {
			r.res <- nil
		}
		return
	}

	if len(batch) == 1 {
		if err != errAlreadyExists {
			err = fmt.Errorf("add port failed: %v", err)
		}
		batch[0].res <- err
		return
	}

	log.Debug("ovs batch of %v port adds failed, splitting: %v", len(batch), err)

	mid := len(batch) / 2
	o.commit(batch[:mid])
	o.commit(batch[mid:])
}
