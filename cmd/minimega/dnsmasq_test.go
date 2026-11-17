package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// fake dnsmasq: writes its pid to --pid-file and sleeps until SIGTERM.
const fakeDnsmasq = `#!/bin/sh
for a in "$@"; do case "$a" in --pid-file=*) echo $$ > "${a#--pid-file=}";; esac; done
exec sleep 60
`

func runDnsmasq(t *testing.T, cmd string) *minicli.Response {
	t.Helper()
	resp := &minicli.Response{}
	if err := cliDnsmasq(nil, minicli.MustCompile(cmd), resp); err != nil {
		t.Fatalf("%q: %v", cmd, err)
	}
	return resp
}

// "dnsmasq kill all" races with the waiter goroutines deleting from the map
// as the killed servers exit.
func TestDnsmasqKillAllRace(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dnsmasq"), []byte(fakeDnsmasq), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	oldBase := *f_base
	*f_base = dir
	defer func() { *f_base = oldBase }()

	for i := 0; i < 5; i++ {
		runDnsmasq(t, fmt.Sprintf("dnsmasq start 10.0.0.%d 10.0.1.1 10.0.1.9", i+1))
	}

	// wait for the fake servers to write their pid files
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ok := true
		for _, row := range runDnsmasq(t, "dnsmasq").Tabular {
			if row[5] == "-1" {
				ok = false
			}
		}
		if ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	runDnsmasq(t, "dnsmasq kill all")

	for time.Now().Before(deadline.Add(5 * time.Second)) {
		if len(runDnsmasq(t, "dnsmasq").Tabular) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("servers never exited")
}
