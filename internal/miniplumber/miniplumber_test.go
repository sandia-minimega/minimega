package miniplumber

import (
	"strconv"
	"sync"

	"fmt"
	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

// A late QUERY_RESPONSE for an already-unregistered TID must not kill the
// message loop: later messages still have to be delivered to readers.
func TestHandleMessagesSurvivesUnknownTID(t *testing.T) {
	p := New(nil)
	go p.handleMessages()

	r := p.NewReader("pipe")

	p.Messages <- &meshage.Message{Body: Message{Type: MESSAGE_QUERY_RESPONSE, TID: 12345}}
	p.Messages <- &meshage.Message{Body: Message{
		Type: MESSAGE_FORWARD,
		Pipe: "pipe",
		Data: map[int64]string{SCHEDULE_ALL: "hello"},
	}}

	select {
	case v := <-r.C:
		if v != "hello\n" {
			t.Fatalf("got %q", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("message loop died after QUERY_RESPONSE for unknown TID")
	}
}

// newTID/unregisterTID and Pipes/NewReader are used from many goroutines;
// run under -race.
func TestConcurrentTIDsAndPipes(t *testing.T) {
	p := New(nil)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				p.unregisterTID(p.newTID())
			}
		}()
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				p.NewReader("pipe" + strconv.Itoa(i*1000+j)).Close()
				_ = p.Pipes()
			}
		}(i)
	}
	wg.Wait()
}
