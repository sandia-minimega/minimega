package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func resetBackground() {
	backgroundProcessesRWLock.Lock()
	backgroundProcesses = make(map[int]*BackgroundProcess)
	backgroundProcessesRWLock.Unlock()
}

func runBG(t *testing.T, f func(*minicli.Command, chan<- minicli.Responses), c *minicli.Command) minicli.Responses {
	t.Helper()
	ch := make(chan minicli.Responses, 1)
	f(c, ch)
	return <-ch
}

// A command that cannot be exec'd (no shebang) must not leave an entry that
// crashes background-status.
func TestBackgroundStatusUnstartableCommand(t *testing.T) {
	resetBackground()
	defer resetBackground()

	script := filepath.Join(t.TempDir(), "noshebang")
	if err := os.WriteFile(script, []byte("echo hi\n"), 0755); err != nil {
		t.Fatal(err)
	}

	runBG(t, cliBackground, &minicli.Command{ListArgs: map[string][]string{"command": {script}}})

	// all-entries form must not panic
	runBG(t, func(c *minicli.Command, ch chan<- minicli.Responses) { cliBackgroundStatusAll(ch) }, nil)
	// by-id form must not panic either
	runBG(t, cliBackgroundStatus, &minicli.Command{StringArgs: map[string]string{"id": "1"}})
}

// clear/flush must hold the lock while touching the map.
func TestBackgroundClearRace(t *testing.T) {
	resetBackground()
	defer resetBackground()

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			backgroundProcessesRWLock.Lock()
			backgroundProcesses[i] = &BackgroundProcess{ID: i, TimeEnd: time.Now()}
			backgroundProcessesRWLock.Unlock()
		}
	}()

	for i := 0; i < 200; i++ {
		runBG(t, cliClearBackgroundStatus, nil)
		flushOldBackgroundStatus()
	}
	close(stop)
	wg.Wait()
}
