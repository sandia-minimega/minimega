package minipager

import (
	"io"
	"os"
	"strings"
	"testing"
)

func capture(t *testing.T, f func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	f()
	os.Stdout = old
	w.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestRunPagerWithArgs(t *testing.T) {
	t.Setenv("PAGER", "tr a-z A-Z")
	if got := capture(t, func() { runPager("hello\n") }); !strings.Contains(got, "HELLO") {
		t.Fatalf("pager with args not run, got %q", got)
	}
}

func TestRunPagerMissingFallsBack(t *testing.T) {
	t.Setenv("PAGER", "/nonexistent/pager")
	if got := capture(t, func() { runPager("hello\n") }); !strings.Contains(got, "hello") {
		t.Fatalf("output lost when pager fails, got %q", got)
	}
}
