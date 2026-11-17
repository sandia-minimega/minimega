// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package miniplumber

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os/exec"
	"sort"
	"sync"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/meshage"
)

func init() {
	// as minimega does for the messages it sends over meshage
	gob.Register(Message{})
}

// fakeMesh connects plumbers in this process the way meshage connects the
// plumbers of the hosts in a mesh.
type fakeMesh struct {
	t *testing.T

	lock  sync.Mutex
	nodes map[string]*fakeNode
}

// fakeNode is the mesh that one plumber sees.
type fakeNode struct {
	name string
	mesh *fakeMesh
	p    *Plumber
}

func newFakeMesh(t *testing.T) *fakeMesh {
	return &fakeMesh{t: t, nodes: make(map[string]*fakeNode)}
}

// add adds a host to the mesh and returns its plumber.
func (m *fakeMesh) add(name string) *Plumber {
	m.lock.Lock()
	defer m.lock.Unlock()

	n := &fakeNode{name: name, mesh: m}
	n.p = newPlumber(n)
	m.nodes[name] = n

	return n.p
}

func (n *fakeNode) Name() string {
	return n.name
}

func (n *fakeNode) Set(recipients []string, body interface{}) ([]string, error) {
	// what the recipients decode: a copy, with a Message as its body however
	// the sender passed it
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&meshage.Message{Source: n.name, Body: body}); err != nil {
		return nil, err
	}

	n.mesh.lock.Lock()
	defer n.mesh.lock.Unlock()

	var sent []string

	for _, r := range recipients {
		to, ok := n.mesh.nodes[r]
		if !ok || to == n {
			return sent, fmt.Errorf("no route to %v", r)
		}

		var m meshage.Message
		if err := gob.NewDecoder(bytes.NewReader(buf.Bytes())).Decode(&m); err != nil {
			return sent, err
		}

		// like meshage, don't wait for the plumber to handle the message
		select {
		case to.p.Messages <- &m:
			sent = append(sent, r)
		default:
			n.mesh.t.Errorf("%v has too many messages waiting", r)
		}
	}

	return sent, nil
}

func (n *fakeNode) Broadcast(body interface{}) ([]string, error) {
	n.mesh.lock.Lock()
	var recipients []string
	for name := range n.mesh.nodes {
		if name != n.name {
			recipients = append(recipients, name)
		}
	}
	n.mesh.lock.Unlock()

	sort.Strings(recipients)

	return n.Set(recipients, body)
}

// total waits until the readers have received n messages between them, and
// returns how many each of them received.
func total(t *testing.T, n int, readers ...*collector) []int {
	t.Helper()

	counts := func() ([]int, int) {
		var res []int
		var sum int
		for _, c := range readers {
			res = append(res, len(c.Lines()))
			sum += len(c.Lines())
		}
		return res, sum
	}

	eventually(t, fmt.Sprintf("%d messages", n), func() bool {
		_, sum := counts()
		return sum >= n
	})

	res, sum := counts()
	if sum != n {
		t.Fatalf("readers got %v messages, want %d in total", res, n)
	}

	return res
}

// TestMeshWrite writes to a pipe that has readers on other hosts.
func TestMeshWrite(t *testing.T) {
	mesh := newFakeMesh(t)
	a, b, c, d := mesh.add("a"), mesh.add("b"), mesh.add("c"), mesh.add("d")

	ra := collect(a.NewReader("foo"))
	rb := collect(b.NewReader("foo"))
	rc := collect(c.NewReader("foo"))
	// a reader of another pipe
	other := collect(b.NewReader("bar"))

	// writes on different hosts are not ordered, so wait for the first ones
	write(t, a, "foo", "one", "two")
	for _, r := range []*collector{ra, rb, rc} {
		r.expect(t, "one\n", "two\n")
	}

	write(t, c, "foo", "three")
	for _, r := range []*collector{ra, rb, rc} {
		r.expect(t, "one\n", "two\n", "three\n")
	}

	if got := other.Lines(); len(got) != 0 {
		t.Errorf("reader of another pipe got %q", got)
	}
	// a host that doesn't use the pipe ignores what is forwarded to it
	if got := d.Pipes(); len(got) != 0 {
		t.Errorf("d has %d pipes, want none", len(got))
	}
}

// TestMeshRoundRobin checks that a message goes to one reader in the mesh,
// and to each reader in turn.
func TestMeshRoundRobin(t *testing.T) {
	mesh := newFakeMesh(t)
	a, b, c := mesh.add("a"), mesh.add("b"), mesh.add("c")

	a.Mode("foo", MODE_RR)

	ra := collect(a.NewReader("foo"))
	rb1 := collect(b.NewReader("foo"))
	rb2 := collect(b.NewReader("foo"))
	rc := collect(c.NewReader("foo"))

	var values []string
	for i := 0; i < 12; i++ {
		values = append(values, fmt.Sprintf("message %d", i))
	}
	write(t, a, "foo", values...)

	for i, n := range total(t, 12, ra, rb1, rb2, rc) {
		if n != 3 {
			t.Errorf("reader %d got %d messages, want 3", i, n)
		}
	}
}

// TestMeshRandom checks that a message goes to one reader in the mesh.
func TestMeshRandom(t *testing.T) {
	mesh := newFakeMesh(t)
	a, b := mesh.add("a"), mesh.add("b")

	a.Mode("foo", MODE_RND)

	ra := collect(a.NewReader("foo"))
	rb := collect(b.NewReader("foo"))

	var values []string
	for i := 0; i < 100; i++ {
		values = append(values, fmt.Sprintf("message %d", i))
	}
	write(t, a, "foo", values...)

	for i, n := range total(t, 100, ra, rb) {
		if n == 0 {
			t.Errorf("reader %d got none of 100 messages", i)
		}
	}
}

// TestMeshSchedulersDontWaitForEachOther writes to a round-robin pipe from two
// hosts at once: each needs the other's readers to pick one.
func TestMeshSchedulersDontWaitForEachOther(t *testing.T) {
	mesh := newFakeMesh(t)
	a, b := mesh.add("a"), mesh.add("b")

	a.Mode("foo", MODE_RR)
	b.Mode("foo", MODE_RR)

	ra := collect(a.NewReader("foo"))
	rb := collect(b.NewReader("foo"))

	const n = 50

	var wg sync.WaitGroup
	for _, p := range []*Plumber{a, b} {
		wg.Add(1)
		go func(p *Plumber) {
			defer wg.Done()

			w, done := p.NewWriterDone("foo")
			for i := 0; i < n; i++ {
				w <- fmt.Sprintf("message %d", i)
			}
			close(w)
			<-done
		}(p)
	}
	wg.Wait()

	total(t, 2*n, ra, rb)
}

// TestMeshVia checks that the host a via was set on runs it, once for each
// reader in the mesh, whichever host the value is written on.
func TestMeshVia(t *testing.T) {
	if _, err := exec.LookPath("tr"); err != nil {
		t.Skip("tr not found")
	}

	mesh := newFakeMesh(t)
	a, b, c := mesh.add("a"), mesh.add("b"), mesh.add("c")

	ra := collect(a.NewReader("foo"))
	rb := collect(b.NewReader("foo"))
	rc := collect(c.NewReader("foo"))

	b.Via("foo", []string{"tr", "a-z", "A-Z"})

	// the other hosts hand their writes to b
	for _, p := range []*Plumber{a, c} {
		pp := getPipe(t, p, "foo")
		eventually(t, "the via host", func() bool {
			pp.lock.Lock()
			defer pp.lock.Unlock()

			return pp.viaHost == "b"
		})
		if got := pp.GetVia(); got != "" {
			t.Errorf("via = %q on a host that doesn't run it", got)
		}
	}

	write(t, a, "foo", "from a")
	for _, r := range []*collector{ra, rb, rc} {
		r.expect(t, "FROM A\n")
	}

	write(t, b, "foo", "from b")
	for _, r := range []*collector{ra, rb, rc} {
		r.expect(t, "FROM A\n", "FROM B\n")
	}

	// without the via, every host writes for itself again
	b.Via("foo", nil)
	for _, p := range []*Plumber{a, c} {
		pp := getPipe(t, p, "foo")
		eventually(t, "no via host", func() bool {
			pp.lock.Lock()
			defer pp.lock.Unlock()

			return pp.viaHost == p.node.Name()
		})
	}

	write(t, c, "foo", "from c")
	for _, r := range []*collector{ra, rb, rc} {
		r.expect(t, "FROM A\n", "FROM B\n", "from c\n")
	}
}

// TestMeshViaRoundRobin checks that a pipe with a via still delivers a
// message to one reader only.
func TestMeshViaRoundRobin(t *testing.T) {
	if _, err := exec.LookPath("tr"); err != nil {
		t.Skip("tr not found")
	}

	mesh := newFakeMesh(t)
	a, b, c := mesh.add("a"), mesh.add("b"), mesh.add("c")

	ra := collect(a.NewReader("foo"))
	rb := collect(b.NewReader("foo"))
	rc := collect(c.NewReader("foo"))

	// the host that runs the via picks the reader
	b.Via("foo", []string{"tr", "a-z", "A-Z"})
	b.Mode("foo", MODE_RR)

	pp := getPipe(t, a, "foo")
	eventually(t, "the via host", func() bool {
		pp.lock.Lock()
		defer pp.lock.Unlock()

		return pp.viaHost == "b"
	})

	var values []string
	for i := 0; i < 9; i++ {
		values = append(values, fmt.Sprintf("message %d", i))
	}
	write(t, a, "foo", values...)

	for i, n := range total(t, 9, ra, rb, rc) {
		if n != 3 {
			t.Errorf("reader %d got %d messages, want 3", i, n)
		}
	}
	for _, r := range []*collector{ra, rb, rc} {
		for _, v := range r.Lines() {
			if v[:8] != "MESSAGE " {
				t.Errorf("got %q, which did not go through the via", v)
			}
		}
	}
}

// TestMeshPipelineRoundRobin writes, on one host, to the first pipe of a
// pipeline on another host. The pipeline's last pipe is in round-robin mode,
// so the pipeline waits for the other hosts' readers for every line, while
// the next lines for it arrive.
func TestMeshPipelineRoundRobin(t *testing.T) {
	mesh := newFakeMesh(t)
	a, b := mesh.add("a"), mesh.add("b")

	if err := a.Plumb("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	a.Mode("bar", MODE_RR)

	eventually(t, "the pipeline to start", func() bool {
		if len(a.Pipes()) != 2 {
			return false
		}
		return getPipe(t, a, "foo").NumReaders() == 1 && getPipe(t, a, "bar").NumWriters() == 1
	})

	rb := collect(b.NewReader("bar"))

	lines := numbered("line", 50)
	write(t, b, "foo", lines...)

	rb.expect(t, lines...)
}

// TestMeshViaOrder checks that values written on a host that doesn't run the
// pipe's via reach the readers in the order in which they were written.
func TestMeshViaOrder(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not found")
	}

	mesh := newFakeMesh(t)
	a, b := mesh.add("a"), mesh.add("b")

	ra := collect(a.NewReader("foo"))
	rb := collect(b.NewReader("foo"))

	b.Via("foo", []string{"cat"})

	pp := getPipe(t, a, "foo")
	eventually(t, "the via host", func() bool {
		pp.lock.Lock()
		defer pp.lock.Unlock()

		return pp.viaHost == "b"
	})

	lines := numbered("line", 100)
	write(t, a, "foo", lines...)

	ra.expect(t, lines...)
	rb.expect(t, lines...)
}

// TestMeshReaderDoesNotRead checks that a reader that doesn't take its
// messages only holds up its own pipe: messages from another host for other
// pipes arrive, and the host still learns the other hosts' readers.
func TestMeshReaderDoesNotRead(t *testing.T) {
	mesh := newFakeMesh(t)
	a, b := mesh.add("a"), mesh.add("b")

	// nothing reads from this reader
	stuck := a.NewReader("stuck")
	defer stuck.Close()

	ra := collect(a.NewReader("foo"))
	rb := collect(b.NewReader("bar"))
	a.Mode("bar", MODE_RR)

	// the writes to stuck wait for its reader on a
	w := b.NewWriter("stuck")
	defer close(w)
	for _, v := range numbered("stuck", 3) {
		w <- v
	}

	write(t, b, "foo", "from b")
	ra.expect(t, "from b\n")

	// round-robin: a asks b for its readers, and b's answer has to get
	// through
	write(t, a, "bar", "from a")
	rb.expect(t, "from a\n")
}
