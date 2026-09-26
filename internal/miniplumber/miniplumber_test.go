package miniplumber

import (
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
