// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestHostInfoKeysQemuLast(t *testing.T) {
	if got := hostInfoKeys[len(hostInfoKeys)-1]; got != "qemu" {
		t.Errorf("last host info key = %q, want %q", got, "qemu")
	}
}

func TestHostStatsPrintQemu(t *testing.T) {
	h := &HostStats{QEMU: "11.1.3"}

	if got := h.Print("qemu"); got != "11.1.3" {
		t.Errorf("Print(qemu) = %q, want %q", got, "11.1.3")
	}
}

func TestHostStatsPrintAllKeys(t *testing.T) {
	h := &HostStats{QEMU: "11.1.3"}

	seen := map[string]bool{}
	for _, k := range hostInfoKeys {
		if seen[k] {
			t.Errorf("duplicate host info key %q", k)
		}
		seen[k] = true

		if h.Print(k) == "???" {
			t.Errorf("Print(%q) is not handled", k)
		}
	}
}

func TestHostQemuPattern(t *testing.T) {
	c, err := minicli.Compile("host qemu")
	if err != nil {
		t.Fatal(err)
	}

	if !c.BoolArgs["qemu"] {
		t.Errorf("BoolArgs = %v, want qemu", c.BoolArgs)
	}
}

func TestVersionCache(t *testing.T) {
	calls := 0
	c := &versionCache{probe: func() ([]int, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("kvm not found")
		}
		return []int{11, 1, 3}, nil
	}}

	if got := c.get(); got != "" {
		t.Errorf("first call = %q, want empty string", got)
	}

	if got := c.get(); got != "11.1.3" {
		t.Errorf("second call = %q, want %q", got, "11.1.3")
	}

	if got := c.get(); got != "11.1.3" {
		t.Errorf("third call = %q, want %q", got, "11.1.3")
	}

	if calls != 2 {
		t.Errorf("version probe ran %d times, want 2", calls)
	}
}

func TestVersionCacheSharesFailedProbe(t *testing.T) {
	const callers = 5

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	joined := make(chan struct{}, callers)

	c := &versionCache{
		probe: func() ([]int, error) {
			if calls.Add(1) == 1 {
				close(started)
				<-release
			}
			return nil, errors.New("kvm -version timed out")
		},
		joined: func() { joined <- struct{}{} },
	}

	var wg sync.WaitGroup
	results := make(chan string, callers)
	get := func() {
		defer wg.Done()
		results <- c.get()
	}

	wg.Add(1)
	go get()
	<-started

	wg.Add(callers - 1)
	for i := 1; i < callers; i++ {
		go get()
	}

	timeout := time.After(10 * time.Second)
	for i := 1; i < callers; i++ {
		select {
		case <-joined:
		case <-timeout:
			close(release)
			t.Fatalf("%d of %d callers joined the running probe", i-1, callers-1)
		}
	}
	close(release)
	wg.Wait()
	close(results)

	for got := range results {
		if got != "" {
			t.Errorf("get() = %q, want empty string", got)
		}
	}

	if n := calls.Load(); n != 1 {
		t.Errorf("version probe ran %d times for overlapping calls, want 1", n)
	}

	c.get()
	if n := calls.Load(); n != 2 {
		t.Errorf("version probe ran %d times after a failure, want 2", n)
	}
}

func TestParseQemuVersion(t *testing.T) {
	tests := []struct {
		out  string
		want string
	}{
		{"QEMU emulator version 11.1.3\nCopyright (c) 2003-2025 Fabrice Bellard and the QEMU Project developers\n", "11.1.3"},
		{"QEMU emulator version 7.2.15 (Debian 1:7.2+dfsg-7+deb12u12)\n", "7.2.15"},
		{"QEMU emulator version 8.2.2 (Debian 1:8.2.2+ds-0ubuntu1.4)\n", "8.2.2"},
		{"QEMU emulator version 9.0.0-rc1\n", "9.0.0"},
	}

	for _, tt := range tests {
		v, err := parseQemuVersion(tt.out)
		if err != nil {
			t.Errorf("parseQemuVersion(%q) error: %v", tt.out, err)
			continue
		}

		if got := printVersion(v); got != tt.want {
			t.Errorf("parseQemuVersion(%q) = %q, want %q", tt.out, got, tt.want)
		}
	}
}

func TestParseQemuVersionError(t *testing.T) {
	for _, out := range []string{"", "kvm: not found", "QEMU emulator version unknown"} {
		if v, err := parseQemuVersion(out); err == nil {
			t.Errorf("parseQemuVersion(%q) = %v, want error", out, v)
		}
	}
}

func TestProcessWrapperContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := processWrapperContext(ctx, "sleep", "10"); err == nil {
		t.Error("processWrapperContext returned no error after the timeout")
	}

	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("processWrapperContext returned after %v, want the timeout", d)
	}
}
