package miniplumber

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/meshage"
)

const orderCount = 2000

func TestPlumbFileTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output")
	plumber := New(nil)

	if err := plumber.Plumb("printf hello", "file://"+path); err != nil {
		t.Fatalf("Plumb returned error: %v", err)
	}

	waitForPipeline(t, plumber)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("file contents = %q, want %q", data, "hello\n")
	}

	if err := plumber.Plumb("printf world", "file://"+path); err != nil {
		t.Fatalf("Plumb returned error: %v", err)
	}

	waitForPipeline(t, plumber)

	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(data) != "hello\nworld\n" {
		t.Fatalf("file contents = %q, want %q", data, "hello\nworld\n")
	}
}

func TestPlumbFileTerminalPathWithQuote(t *testing.T) {
	path := filepath.Join(t.TempDir(), `a"b`)
	plumber := New(nil)

	if err := plumber.Plumb("printf hello", "file://"+path); err != nil {
		t.Fatalf("Plumb returned error: %v", err)
	}

	waitForPipeline(t, plumber)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("file contents = %q, want %q", data, "hello\n")
	}
}

func TestPipelinesDuringRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output")
	plumber := New(nil)

	stop := make(chan struct{})
	polled := make(chan struct{})
	go func() {
		defer close(polled)
		for {
			select {
			case <-stop:
				return
			default:
				plumber.Pipelines()
			}
		}
	}()
	defer func() {
		close(stop)
		<-polled
	}()

	for i := 0; i < 20; i++ {
		if err := plumber.Plumb(fmt.Sprintf("printf %d", i), "file://"+path); err != nil {
			t.Fatalf("Plumb returned error: %v", err)
		}
	}

	waitForPipeline(t, plumber)
}

func TestPlumbRejectsInvalidFileTerminalPosition(t *testing.T) {
	tests := []struct {
		name       string
		production []string
	}{
		{
			name:       "source",
			production: []string{"file:///tmp/output"},
		},
		{
			name:       "non-final",
			production: []string{"printf hello", "file:///tmp/output", "cat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := New(nil).Plumb(tt.production...); err == nil {
				t.Fatal("Plumb returned nil error")
			}
		})
	}
}

func waitForPipeline(t *testing.T, plumber *Plumber) {
	t.Helper()

	deadline := time.After(TIMEOUT)
	for {
		if len(plumber.Pipelines()) == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pipeline")
		case <-time.After(time.Millisecond):
		}
	}
}

// readOrdered reads count values from r and reports an error unless they are
// 0..count-1. It keeps reading after a mismatch so writers are not blocked.
func readOrdered(t *testing.T, r *Reader, count int) {
	t.Helper()

	var wrong int
	for want := 0; want < count; want++ {
		select {
		case v := <-r.C:
			if v != strconv.Itoa(want)+"\n" {
				wrong++
			}
		case <-time.After(TIMEOUT):
			t.Errorf("reader %v timed out waiting for value %d", r.ID, want)
			return
		}
	}
	if wrong > 0 {
		t.Errorf("reader %v got %d of %d values out of order", r.ID, wrong, count)
	}
}

func waitForWriters(t *testing.T, plumber *Plumber, pipe string) {
	t.Helper()

	deadline := time.After(TIMEOUT)
	for {
		for _, pp := range plumber.Pipes() {
			if pp.Name() == pipe && pp.NumWriters() == 0 {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for writers on %v", pipe)
		case <-time.After(time.Millisecond):
		}
	}
}

// TestWriteOrder checks that successive Writes reach readers in order.
func TestWriteOrder(t *testing.T) {
	plumber := New(nil)
	readers := []*Reader{plumber.NewReader("foo"), plumber.NewReader("foo")}

	go func() {
		for i := 0; i < orderCount; i++ {
			plumber.Write("foo", strconv.Itoa(i))
		}
	}()

	var wg sync.WaitGroup
	for _, r := range readers {
		wg.Add(1)
		go func(r *Reader) {
			defer wg.Done()
			readOrdered(t, r, orderCount)
		}(r)
	}
	wg.Wait()
	if t.Failed() {
		return
	}

	waitForWriters(t, plumber, "foo")

	if err := plumber.PipeDelete("foo"); err != nil {
		t.Fatalf("PipeDelete returned error: %v", err)
	}
}

// TestWriteReleasesBacklog checks that a pipe does not keep the array of a
// drained Write backlog.
func TestWriteReleasesBacklog(t *testing.T) {
	plumber := New(nil)
	r := plumber.NewReader("foo")

	// nothing reads r yet, so the first value blocks and the rest queue up
	for i := 0; i < 101; i++ {
		plumber.Write("foo", strconv.Itoa(i))
	}
	readOrdered(t, r, 101)
	if t.Failed() {
		return
	}

	plumber.lock.Lock()
	pp := plumber.pipes["foo"]
	plumber.lock.Unlock()

	deadline := time.After(TIMEOUT)
	for {
		pp.lock.Lock()
		writing, held := pp.writing, cap(pp.writes)
		pp.lock.Unlock()
		if !writing {
			if held != 0 {
				t.Fatalf("idle pipe keeps an array for %d values", held)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("write goroutine did not stop when empty")
		case <-time.After(time.Millisecond):
		}
	}
}

// TestWriteOrderManyPipes checks ordering per pipe when several pipes are
// written concurrently.
func TestWriteOrderManyPipes(t *testing.T) {
	plumber := New(nil)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		pipe := fmt.Sprintf("pipe%d", i)
		r := plumber.NewReader(pipe)

		go func() {
			for j := 0; j < orderCount/4; j++ {
				plumber.Write(pipe, strconv.Itoa(j))
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			readOrdered(t, r, orderCount/4)
		}()
	}
	wg.Wait()
}

// TestWriteOrderThroughPipeline checks that Writes keep their order through
// a pipeline into a file terminal.
func TestWriteOrderThroughPipeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output")
	plumber := New(nil)

	if err := plumber.Plumb("foo", "file://"+path); err != nil {
		t.Fatalf("Plumb returned error: %v", err)
	}

	// wait for the pipeline to register its reader
	deadline := time.After(TIMEOUT)
	for registered := false; !registered; {
		for _, pp := range plumber.Pipes() {
			if pp.Name() == "foo" && pp.NumReaders() == 1 {
				registered = true
			}
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pipeline reader")
		case <-time.After(time.Millisecond):
		}
	}

	var want strings.Builder
	for i := 0; i < orderCount; i++ {
		plumber.Write("foo", strconv.Itoa(i))
		fmt.Fprintf(&want, "%d\n", i)
	}

	deadline = time.After(TIMEOUT)
	for {
		data, _ := os.ReadFile(path)
		if len(data) >= want.Len() {
			if string(data) != want.String() {
				t.Errorf("file terminal received values out of order")
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("file terminal has %d of %d bytes", len(data), want.Len())
		case <-time.After(time.Millisecond):
		}
	}

	if err := plumber.PipelineDeleteAll(); err != nil {
		t.Fatalf("PipelineDeleteAll returned error: %v", err)
	}
	waitForPipeline(t, plumber)
}

// TestHandleMessagesAfterInvalidTID checks that a late query response does not
// stop the plumber from handling later meshage messages.
func TestHandleMessagesAfterInvalidTID(t *testing.T) {
	plumber := New(nil)
	go plumber.handleMessages()

	r := plumber.NewReader("foo")

	plumber.Messages <- &meshage.Message{Body: Message{
		Type: MESSAGE_QUERY_RESPONSE,
		TID:  42,
	}}
	plumber.Messages <- &meshage.Message{Body: Message{
		Type: MESSAGE_FORWARD,
		Pipe: "foo",
		Data: map[int64]string{SCHEDULE_ALL: "hello"},
	}}

	select {
	case v := <-r.C:
		if v != "hello\n" {
			t.Fatalf("reader got %q, want %q", v, "hello\n")
		}
	case <-time.After(TIMEOUT):
		t.Fatal("forwarded message was not delivered after an invalid TID")
	}
}

// TestPipesDuringNewReader runs Pipes alongside pipe creation for the race
// detector.
func TestPipesDuringNewReader(t *testing.T) {
	plumber := New(nil)

	stop := make(chan struct{})
	polled := make(chan struct{})
	go func() {
		defer close(polled)
		for {
			select {
			case <-stop:
				return
			default:
				plumber.Pipes()
			}
		}
	}()

	for i := 0; i < 100; i++ {
		plumber.NewReader(fmt.Sprintf("pipe%d", i))
	}

	close(stop)
	<-polled
}

// TestWriteDuringNewReader runs Write alongside readers joining and leaving the
// same pipe for the race detector.
func TestWriteDuringNewReader(t *testing.T) {
	plumber := New(nil)
	r := plumber.NewReader("foo")

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-r.C:
			}
		}
	}()

	for i := 0; i < 100; i++ {
		plumber.Write("foo", strconv.Itoa(i))
		plumber.NewReader("foo").Close()
	}

	close(stop)
	<-done
	r.Close()
}

// readPipeline reads count values from r and fails unless they are 0..count-1,
// for tests that feed a pipeline.
func readPipeline(t *testing.T, r *Reader, count int) {
	t.Helper()

	for want := 0; want < count; want++ {
		select {
		case v := <-r.C:
			if v != strconv.Itoa(want)+"\n" {
				t.Fatalf("got %q, want %d", v, want)
			}
		case <-time.After(TIMEOUT):
			t.Fatalf("pipeline stalled after %d of %d values", want, count)
		}
	}
}

// plumbAndWait creates a pipeline from pipe src to pipe dst and waits for it to
// read src.
func plumbAndWait(t *testing.T, plumber *Plumber, src, dst string) {
	t.Helper()

	if err := plumber.Plumb(src, dst); err != nil {
		t.Fatalf("Plumb returned error: %v", err)
	}

	deadline := time.After(TIMEOUT)
	for {
		for _, pp := range plumber.Pipes() {
			if pp.Name() == src && pp.NumReaders() == 1 {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pipeline reader")
		case <-time.After(time.Millisecond):
		}
	}
}

// TestPipelineFromWrites checks that a pipeline between two pipes keeps up
// with Writes to its source. Delivering to the pipeline while holding the pipe
// lock used to deadlock with the pipeline's own writer.
func TestPipelineFromWrites(t *testing.T) {
	plumber := New(nil)
	out := plumber.NewReader("bar")
	plumbAndWait(t, plumber, "foo", "bar")

	go func() {
		for i := 0; i < 200; i++ {
			plumber.Write("foo", strconv.Itoa(i))
		}
	}()

	readPipeline(t, out, 200)

	if err := plumber.PipelineDeleteAll(); err != nil {
		t.Fatalf("PipelineDeleteAll returned error: %v", err)
	}
	waitForPipeline(t, plumber)
}

// TestPipelineFromMesh checks that a pipeline keeps up with values forwarded
// from another node. Delivering them while holding the plumber lock used to
// deadlock with the pipeline's own writer.
func TestPipelineFromMesh(t *testing.T) {
	plumber := New(nil)
	go plumber.handleMessages()

	out := plumber.NewReader("bar")
	plumbAndWait(t, plumber, "foo", "bar")

	go func() {
		for i := 0; i < 200; i++ {
			plumber.Messages <- &meshage.Message{Body: Message{
				Type: MESSAGE_FORWARD,
				Pipe: "foo",
				Data: map[int64]string{SCHEDULE_ALL: strconv.Itoa(i)},
			}}
		}
	}()

	readPipeline(t, out, 200)

	if err := plumber.PipelineDeleteAll(); err != nil {
		t.Fatalf("PipelineDeleteAll returned error: %v", err)
	}
	waitForPipeline(t, plumber)
}

// TestPipeDeleteWithQueuedWrites checks that values queued by Write do not
// keep a pipe from being deleted, and that its readers are closed.
func TestPipeDeleteWithQueuedWrites(t *testing.T) {
	plumber := New(nil)
	r := plumber.NewReader("foo")

	// nothing reads r, so the first value blocks and the rest stay queued
	for i := 0; i < 10; i++ {
		plumber.Write("foo", strconv.Itoa(i))
	}

	if err := plumber.PipeDelete("foo"); err != nil {
		t.Fatalf("PipeDelete returned error: %v", err)
	}

	select {
	case <-r.Done:
	case <-time.After(TIMEOUT):
		t.Fatal("reader was not closed")
	}

	// the queue is dropped, so the pipe is not recreated
	deadline := time.After(TIMEOUT)
	for {
		if len(plumber.Pipes()) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("pipe was recreated after PipeDelete")
		case <-time.After(time.Millisecond):
		}
	}
}
