// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package miniplumber

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/meshage"
)

// collector drains a Reader in the background, so that writes to its pipe
// never block on it.
type collector struct {
	r *Reader

	lock  sync.Mutex
	lines []string

	// closed when r.C is closed
	done chan struct{}
}

func collect(r *Reader) *collector {
	c := &collector{r: r, done: make(chan struct{})}

	go func() {
		defer close(c.done)

		for v := range r.C {
			c.lock.Lock()
			c.lines = append(c.lines, v)
			c.lock.Unlock()
		}
	}()

	return c
}

func (c *collector) Lines() []string {
	c.lock.Lock()
	defer c.lock.Unlock()

	return append([]string{}, c.lines...)
}

// expect waits until the reader has received exactly want. A message counts
// as delivered once the collector takes it, slightly before it shows in Lines.
func (c *collector) expect(t *testing.T, want ...string) {
	t.Helper()

	deadline := time.Now().Add(TIMEOUT)
	for {
		got := c.Lines()
		if reflect.DeepEqual(got, want) {
			return
		}
		if len(got) > len(want) || time.Now().After(deadline) {
			t.Fatalf("got %q, want %q", got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// count waits until the reader has received n messages.
func (c *collector) count(t *testing.T, n int) {
	t.Helper()

	eventually(t, fmt.Sprintf("%d messages", n), func() bool {
		return len(c.Lines()) >= n
	})
	if got := c.Lines(); len(got) != n {
		t.Fatalf("got %d messages (%q), want %d", len(got), got, n)
	}
}

// write sends values on a new writer and returns once they are all written to
// the pipe.
func write(t *testing.T, p *Plumber, pipe string, values ...string) {
	t.Helper()

	w, done := p.NewWriterDone(pipe)
	for _, v := range values {
		w <- v
	}
	close(w)

	wait(t, done, "writer to finish")
}

func wait(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-c:
	case <-time.After(TIMEOUT):
		t.Fatalf("timed out waiting for %v", what)
	}
}

// eventually polls until fn returns true.
func eventually(t *testing.T, what string, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(TIMEOUT)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %v", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func getPipe(t *testing.T, p *Plumber, name string) *Pipe {
	t.Helper()

	for _, pp := range p.Pipes() {
		if pp.Name() == name {
			return pp
		}
	}

	t.Fatalf("no such pipe: %v", name)
	return nil
}

func TestWriteDeliversToAllReaders(t *testing.T) {
	p := New(nil)

	a := collect(p.NewReader("foo"))
	b := collect(p.NewReader("foo"))

	pp := getPipe(t, p, "foo")
	if got := pp.NumReaders(); got != 2 {
		t.Fatalf("readers = %v, want 2", got)
	}
	if got := pp.Mode(); got != "all" {
		t.Fatalf("mode = %q, want all", got)
	}

	// a newline is added if there isn't one
	write(t, p, "foo", "one", "two\n")

	a.expect(t, "one\n", "two\n")
	b.expect(t, "one\n", "two\n")

	if got := pp.NumMessages(); got != 2 {
		t.Errorf("messages = %v, want 2", got)
	}
	if got := pp.Last(); got != "two\n" {
		t.Errorf("last = %q, want %q", got, "two\n")
	}
	if got := pp.NumWriters(); got != 0 {
		t.Errorf("writers = %v, want 0 after the writer is done", got)
	}
}

// TestWriteWithoutReaders checks that a message written to a pipe without
// readers is discarded, not kept for a later reader.
func TestWriteWithoutReaders(t *testing.T) {
	p := New(nil)

	p.Write("foo", "dropped")
	eventually(t, "the write", func() bool {
		return getPipe(t, p, "foo").NumMessages() == 1
	})

	c := collect(p.NewReader("foo"))
	write(t, p, "foo", "kept")

	c.expect(t, "kept\n")
}

// TestNewWriterDone checks that the done channel is closed only when the
// writer is closed and its last message has been delivered.
func TestNewWriterDone(t *testing.T) {
	p := New(nil)

	r := p.NewReader("foo")
	w, done := p.NewWriterDone("foo")

	if got := getPipe(t, p, "foo").NumWriters(); got != 1 {
		t.Fatalf("writers = %v, want 1", got)
	}

	w <- "hello"
	close(w)

	// nothing reads the message yet, so the writer cannot be done
	select {
	case <-done:
		t.Fatal("writer done before its message was delivered")
	case <-time.After(50 * time.Millisecond):
	}

	if got := <-r.C; got != "hello\n" {
		t.Fatalf("got %q, want %q", got, "hello\n")
	}

	wait(t, done, "writer to finish")

	if got := getPipe(t, p, "foo").NumWriters(); got != 0 {
		t.Fatalf("writers = %v, want 0", got)
	}
}

// numbered returns n distinct messages.
func numbered(prefix string, n int) []string {
	var res []string
	for i := 0; i < n; i++ {
		res = append(res, fmt.Sprintf("%v %d\n", prefix, i))
	}
	return res
}

func TestModeRoundRobin(t *testing.T) {
	p := New(nil)
	p.Mode("foo", MODE_RR)

	a := collect(p.NewReader("foo"))
	b := collect(p.NewReader("foo"))

	pp := getPipe(t, p, "foo")
	if got := pp.Mode(); got != "round-robin" {
		t.Fatalf("mode = %q, want round-robin", got)
	}

	write(t, p, "foo", "1", "2", "3", "4")

	// each message goes to exactly one reader, and the readers take turns
	a.count(t, 2)
	b.count(t, 2)
	if got := pp.NumMessages(); got != 4 {
		t.Errorf("messages = %v, want 4", got)
	}
}

func TestModeRandom(t *testing.T) {
	p := New(nil)
	p.Mode("foo", MODE_RND)

	a := collect(p.NewReader("foo"))
	b := collect(p.NewReader("foo"))

	if got := getPipe(t, p, "foo").Mode(); got != "random" {
		t.Fatalf("mode = %q, want random", got)
	}

	var values []string
	for i := 0; i < 20; i++ {
		values = append(values, fmt.Sprintf("%d\n", i))
	}
	write(t, p, "foo", values...)

	// each message goes to exactly one reader
	eventually(t, "the messages", func() bool {
		return len(a.Lines())+len(b.Lines()) == len(values)
	})
	seen := map[string]int{}
	for _, v := range append(a.Lines(), b.Lines()...) {
		seen[v]++
	}
	for _, v := range values {
		if seen[v] != 1 {
			t.Errorf("message %q delivered %d times, want 1", v, seen[v])
		}
	}

	// back to delivering to every reader
	p.Mode("foo", MODE_ALL)
	write(t, p, "foo", "all")
	eventually(t, "the message for all readers", func() bool {
		return len(a.Lines())+len(b.Lines()) == len(values)+2
	})
}

// TestModeWithoutReaders checks that a pipe that delivers to one reader
// accepts writes when it has none.
func TestModeWithoutReaders(t *testing.T) {
	p := New(nil)
	p.Mode("foo", MODE_RR)

	write(t, p, "foo", "dropped")

	if got := getPipe(t, p, "foo").NumMessages(); got != 1 {
		t.Fatalf("messages = %v, want 1", got)
	}
}

func TestReaderClose(t *testing.T) {
	p := New(nil)

	a := collect(p.NewReader("foo"))
	b := p.NewReader("foo")

	// a writer blocked on a reader that nothing drains ...
	w, done := p.NewWriterDone("foo")
	w <- "hello"
	close(w)

	// ... is released when that reader is closed, twice is fine
	b.Close()
	b.Close()
	wait(t, done, "writer to finish")

	if _, ok := <-b.C; ok {
		t.Fatal("closed reader received a message")
	}

	pp := getPipe(t, p, "foo")
	eventually(t, "the reader to be removed", func() bool {
		return pp.NumReaders() == 1
	})

	write(t, p, "foo", "again")
	a.expect(t, "hello\n", "again\n")
}

func TestPipeDelete(t *testing.T) {
	p := New(nil)

	if err := p.PipeDelete("foo"); err == nil {
		t.Fatal("deleted a pipe that does not exist")
	}

	c := collect(p.NewReader("foo"))
	w, done := p.NewWriterDone("foo")

	if err := p.PipeDelete("foo"); err == nil {
		t.Fatal("deleted a pipe with an open writer")
	}
	if err := p.PipeDeleteAll(); err == nil {
		t.Fatal("deleted all pipes with an open writer")
	}

	close(w)
	wait(t, done, "writer to finish")

	if err := p.PipeDelete("foo"); err != nil {
		t.Fatalf("PipeDelete: %v", err)
	}

	// deleting a pipe closes its readers
	wait(t, c.done, "reader to be closed")

	if got := len(p.Pipes()); got != 0 {
		t.Fatalf("%d pipes left, want 0", got)
	}

	p.NewReader("a")
	p.NewReader("b")
	if err := p.PipeDeleteAll(); err != nil {
		t.Fatalf("PipeDeleteAll: %v", err)
	}
	if got := len(p.Pipes()); got != 0 {
		t.Fatalf("%d pipes left, want 0", got)
	}
}

// TestNewReaderOnBlockedPipe checks that a new reader on a pipe whose write
// waits for a reader does not block the other pipes.
func TestNewReaderOnBlockedPipe(t *testing.T) {
	p := New(nil)

	stalled := p.NewReader("foo")
	w, done := p.NewWriterDone("foo")
	w <- "hello"
	close(w)

	// not getPipe: Name waits for the pipe lock
	p.lock.Lock()
	pp := p.pipes["foo"]
	p.lock.Unlock()
	eventually(t, "the write to hold the pipe lock", func() bool {
		if pp.lock.TryLock() {
			pp.lock.Unlock()
			return false
		}
		return true
	})

	added := make(chan *Reader, 1)
	go func() {
		added <- p.NewReader("foo")
	}()
	// let NewReader start to wait for the pipe lock
	time.Sleep(50 * time.Millisecond)

	other := make(chan struct{})
	go func() {
		defer close(other)
		p.Write("bar", "hello")
	}()
	wait(t, other, "a write to another pipe")

	stalled.Close()
	wait(t, done, "writer to finish")

	var c *collector
	select {
	case r := <-added:
		c = collect(r)
	case <-time.After(TIMEOUT):
		t.Fatal("timed out waiting for NewReader")
	}

	write(t, p, "foo", "again")
	c.expect(t, "again\n")
}

// TestNewReaderDuringPipeDelete checks that NewReader does not add a reader to
// a pipe that PipeDelete removes while NewReader waits for the pipe lock.
func TestNewReaderDuringPipeDelete(t *testing.T) {
	p := New(nil)
	old := p.NewReader("foo")

	p.lock.Lock()
	pp := p.pipes["foo"]
	p.lock.Unlock()

	// hold the pipe lock like a blocked write
	pp.lock.Lock()

	added := make(chan *Reader, 1)
	go func() {
		added <- p.NewReader("foo")
	}()
	// let NewReader start to wait for the pipe lock
	time.Sleep(50 * time.Millisecond)

	// do what PipeDelete does once it has the pipe lock
	deadline := time.Now().Add(TIMEOUT)
	for !p.lock.TryLock() {
		if time.Now().After(deadline) {
			t.Fatal("NewReader holds the plumber lock while it waits for the pipe lock")
		}
		time.Sleep(time.Millisecond)
	}
	old.Close()
	pp.deleted = true
	delete(p.pipes, "foo")
	p.lock.Unlock()
	pp.lock.Unlock()

	var c *collector
	select {
	case r := <-added:
		c = collect(r)
	case <-time.After(TIMEOUT):
		t.Fatal("timed out waiting for NewReader")
	}

	if getPipe(t, p, "foo") == pp {
		t.Fatal("deleted pipe is listed")
	}
	write(t, p, "foo", "hello")
	c.expect(t, "hello\n")
}

func TestPipesAreSorted(t *testing.T) {
	p := New(nil)

	// every way of naming a pipe creates it
	p.NewReader("d")
	p.Log("c", true)
	p.Mode("b", MODE_ALL)
	p.Write("a", "hello")
	p.Log("c", false)

	var got []string
	for _, pp := range p.Pipes() {
		got = append(got, pp.Name())
	}

	if want := []string{"a", "b", "c", "d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pipes = %v, want %v", got, want)
	}
}

// TestPipesDuringChanges lists pipes while others are created and deleted. Run
// with -race.
func TestPipesDuringChanges(t *testing.T) {
	p := New(nil)

	stop := make(chan struct{})
	listed := make(chan struct{})
	go func() {
		defer close(listed)
		for {
			select {
			case <-stop:
				return
			default:
				for _, pp := range p.Pipes() {
					pp.Name()
				}
			}
		}
	}()

	for i := 0; i < 200; i++ {
		name := fmt.Sprintf("pipe%d", i)
		p.NewReader(name)
		if err := p.PipeDelete(name); err != nil {
			t.Fatal(err)
		}
	}

	close(stop)
	<-listed
}

func TestVia(t *testing.T) {
	if _, err := exec.LookPath("tr"); err != nil {
		t.Skip("tr not found")
	}

	p := New(nil)
	p.Via("foo", []string{"tr", "a-z", "A-Z"})

	pp := getPipe(t, p, "foo")
	if got := pp.GetVia(); got != "tr a-z A-Z" {
		t.Fatalf("via = %q", got)
	}

	c := collect(p.NewReader("foo"))
	write(t, p, "foo", "hello")

	c.expect(t, "HELLO\n")
	// the pipe remembers what was written, not what the via made of it
	if got := pp.Last(); got != "hello\n" {
		t.Errorf("last = %q, want %q", got, "hello\n")
	}

	// a via that cannot run delivers nothing
	p.Via("foo", []string{"/nonexistent/via"})
	write(t, p, "foo", "lost")

	// and without a via the value passes through again
	p.Via("foo", nil)
	write(t, p, "foo", "plain")

	c.expect(t, "HELLO\n", "plain\n")
}

func TestPlumbPipes(t *testing.T) {
	p := New(nil)

	if err := p.Plumb("a", "b", "c"); err != nil {
		t.Fatal(err)
	}
	if err := p.Plumb("a", "b", "c"); err == nil {
		t.Fatal("created the same pipeline twice")
	}

	if got, want := p.Pipelines(), []string{"a b c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pipelines = %v, want %v", got, want)
	}

	// the pipeline reads a and b, and writes b and c
	eventually(t, "the pipeline to start", func() bool {
		if len(p.Pipes()) != 3 {
			return false
		}
		return getPipe(t, p, "a").NumReaders() == 1 &&
			getPipe(t, p, "b").NumReaders() == 1 && getPipe(t, p, "b").NumWriters() == 1 &&
			getPipe(t, p, "c").NumWriters() == 1
	})

	c := collect(p.NewReader("c"))
	write(t, p, "a", "hello")

	c.expect(t, "hello\n")

	if err := p.PipelineDelete("a", "b"); err == nil {
		t.Fatal("deleted a pipeline that does not exist")
	}
	if err := p.PipelineDelete("a", "b", "c"); err != nil {
		t.Fatal(err)
	}
	waitForPipeline(t, p)

	// the pipeline let go of its pipes
	eventually(t, "the pipeline to detach", func() bool {
		return getPipe(t, p, "a").NumReaders() == 0 && getPipe(t, p, "c").NumWriters() == 0
	})
}

func TestPipelineDeleteAll(t *testing.T) {
	p := New(nil)

	for _, production := range [][]string{{"a", "b"}, {"a", "c"}} {
		if err := p.Plumb(production...); err != nil {
			t.Fatal(err)
		}
	}

	if err := p.PipelineDeleteAll(); err != nil {
		t.Fatal(err)
	}
	waitForPipeline(t, p)
}

// TestPlumbProcess plumbs a pipe through a process and into another pipe, like
// the example in the plumb help.
func TestPlumbProcess(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not found")
	}

	p := New(nil)

	if err := p.Plumb("in", "cat", "out"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the pipeline to start", func() bool {
		return len(p.Pipes()) == 2 && getPipe(t, p, "in").NumReaders() == 1
	})

	c := collect(p.NewReader("out"))
	write(t, p, "in", "hello", "world")

	c.expect(t, "hello\n", "world\n")

	// closing the first pipe shuts the pipeline down
	if err := p.PipeDelete("in"); err != nil {
		t.Fatal(err)
	}
	waitForPipeline(t, p)
}

// TestPlumbProcessNotFound checks that a pipeline whose process cannot start
// goes away again.
func TestPlumbProcessNotFound(t *testing.T) {
	p := New(nil)

	// an element that is not in $PATH is a pipe, so use a path that is found
	// but cannot be executed as a program with these arguments
	if err := p.Plumb("in", "/dev/null", "out"); err != nil {
		t.Fatal(err)
	}

	if err := p.PipelineDelete("in", "/dev/null", "out"); err != nil {
		// already gone
		t.Log(err)
	}
	waitForPipeline(t, p)
}

// TestClosedPipeWritesNothingDownstream closes the pipe a pipeline reads from
// and checks that the pipeline does not write an empty message to the next
// pipe on its way out.
func TestClosedPipeWritesNothingDownstream(t *testing.T) {
	for i := 0; i < 200; i++ {
		p := New(nil)

		if err := p.Plumb("a", "b"); err != nil {
			t.Fatal(err)
		}
		eventually(t, "the pipeline to start", func() bool {
			return len(p.Pipes()) == 2 && getPipe(t, p, "a").NumReaders() == 1 && getPipe(t, p, "b").NumWriters() == 1
		})

		if err := p.PipeDelete("a"); err != nil {
			t.Fatal(err)
		}
		waitForPipeline(t, p)

		b := getPipe(t, p, "b")
		eventually(t, "the pipeline to detach", func() bool {
			return b.NumWriters() == 0
		})

		if got := b.NumMessages(); got != 0 {
			t.Fatalf("run %d: %d messages written to b (last %q), want 0", i, got, b.Last())
		}
	}
}

// TestHandleMessages feeds the plumber messages from other hosts.
func TestHandleMessages(t *testing.T) {
	p := New(nil)
	go p.handleMessages()

	send := func(m Message) {
		p.Messages <- &meshage.Message{Body: m}
	}

	a := p.NewReader("foo")
	b := collect(p.NewReader("foo"))

	// a response to a query that is no longer waiting for one is dropped, and
	// must not stop the plumber from handling the messages that follow
	send(Message{Type: MESSAGE_QUERY_RESPONSE, TID: 42})

	// a message forwarded for one reader goes to that reader only
	send(Message{Type: MESSAGE_FORWARD, Pipe: "foo", Data: map[int64]string{a.ID: "for a"}})
	select {
	case got := <-a.C:
		if got != "for a\n" {
			t.Fatalf("got %q, want %q", got, "for a\n")
		}
	case <-time.After(TIMEOUT):
		t.Fatal("timed out waiting for the forwarded message")
	}
	a.Close()

	// a message forwarded for every reader, and for a pipe nobody reads here
	send(Message{Type: MESSAGE_FORWARD, Pipe: "bar", Data: map[int64]string{SCHEDULE_ALL: "nobody"}})
	send(Message{Type: MESSAGE_FORWARD, Pipe: "foo", Data: map[int64]string{SCHEDULE_ALL: "for all"}})

	// a write another host hands to the host that runs the pipe's via
	send(Message{Type: MESSAGE_VIA_WRITE, Pipe: "foo", Value: "via write"})

	// an unknown message type is logged and skipped
	send(Message{Type: -1})

	b.expect(t, "for all\n", "via write\n")
}

func TestFieldsQuoteEscape(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{`a b c`, []string{"a", "b", "c"}},
		{`a b "c d"`, []string{"a", "b", "c d"}},
		{`sed -u "s/foo/moo moo/" x`, []string{"sed", "-u", "s/foo/moo moo/", "x"}},
		{`"foo"`, []string{"foo"}},
		{``, nil},
	}

	for _, tt := range tests {
		if got := fieldsQuoteEscape(`"`, tt.input); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("fieldsQuoteEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	if got := trimQuote("", "foo"); got != "" {
		t.Errorf("trimQuote with nothing to trim = %q, want an empty string", got)
	}
	if got := strings.Join(fieldsQuoteEscape(`'`, `echo 'a b'`), "|"); got != "echo|a b" {
		t.Errorf("single quotes: got %q", got)
	}
}
