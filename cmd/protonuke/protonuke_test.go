package main

import (
	"os"
	"testing"
	"time"
)

// The clients under test loop forever, so the globals they read (hosts, keys,
// flags) are set once here, before any of them starts.
func TestMain(m *testing.M) {
	hosts = map[string]string{"::1": "::1"}
	keys = []string{"::1"}
	*f_mean, *f_max = 2*time.Millisecond, 2*time.Millisecond
	*f_channels = "#a"
	os.Exit(m.Run())
}
