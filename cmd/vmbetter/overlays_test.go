package main

import (
	"os"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/vmconfig"
)

// A missing non-last overlay in a config with no parents must produce an
// error, not an index-out-of-range panic.
func TestOverlaysMissingNoParents(t *testing.T) {
	c := vmconfig.Config{
		Path:     t.TempDir() + "/cfg",
		Overlays: []string{"/nonexistent/a", "/nonexistent/b"},
	}

	err := Overlays(t.TempDir(), c)
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}
