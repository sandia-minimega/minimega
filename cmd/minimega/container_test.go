package main

import (
	"os"
	"path/filepath"
	"testing"
)

// launch() runs on every relaunch of a quit/errored container and the fifos
// from the previous run are still on disk.
func TestMkfifosRelaunch(t *testing.T) {
	dir := t.TempDir()
	for run := 0; run < 2; run++ {
		if err := mkfifos(dir, 2); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}
	for _, n := range []string{"fifo0", "fifo1"} {
		fi, err := os.Stat(filepath.Join(dir, n))
		if err != nil || fi.Mode()&os.ModeNamedPipe == 0 {
			t.Fatalf("%v: %v %v", n, fi, err)
		}
	}
}
