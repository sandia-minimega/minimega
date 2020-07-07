// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var ExternalDependencies = []string{
	"ip",
	"ovs-vsctl",
	"ovs-ofctl",
	"tc",
}

// processWrapper executes the given arg list and returns a combined
// stdout/stderr and any errors. processWrapper blocks until the process exits.
func processWrapper(args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty argument list")
	}

	start := time.Now()
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	stop := time.Now()
	log.Debug("cmd \"%v\" completed in %v, output below:\n %v", strings.Join(args, " "), stop.Sub(start), string(out))

	return string(out), err
}

// cmdTimeout runs the command c and returns a timeout if it doesn't complete
// after time t. If a timeout occurs, cmdTimeout will kill the process. Blocks
// until the process terminates.
func cmdTimeout(c *exec.Cmd, t time.Duration) error {
	log.Debug("cmdTimeout: %v", c)

	start := time.Now()
	if err := c.Start(); err != nil {
		return fmt.Errorf("cmd start: %v", err)
	}

	done := make(chan error)
	go func() {
		done <- c.Wait()
		close(done)
	}()

	select {
	case <-time.After(t):
		log.Debug("killing cmd %v", c)
		err := c.Process.Kill()
		// Receive from done so that we don't leave the goroutine hanging
		err2 := <-done
		// Kill error takes precedence as they should be unexpected
		if err != nil {
			return err
		}
		return err2
	case err := <-done:
		log.Debug("cmd %v completed in %v", c, time.Now().Sub(start))
		return err
	}
}
