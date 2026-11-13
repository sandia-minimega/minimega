// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// CCResult is the outcome of one cc command on one VM.
type CCResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// noExitCode is the ExitCode of a response that recorded none: commands such
// as send and recv do not, and an exec response without one must not read as
// success.
//
// It is not -1: a command that dies on a signal (a blocked wget killed by
// timeout) is recorded with exit code -1, and that is a real result.
const noExitCode = math.MinInt32

func Keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// execResult picks vm's response to an exec. An exec always records an exit
// code, so a response without one is incomplete and must not be read as the
// command having failed (or, for a firewall check, as traffic being blocked).
func execResult(res map[string]CCResult, vm string) (CCResult, error) {
	r, ok := res[vm]
	if !ok {
		return CCResult{}, fmt.Errorf("no cc response from %v (got %v)", vm, Keys(res))
	}
	if r.ExitCode == noExitCode {
		return CCResult{}, fmt.Errorf("cc response from %v has no exit code", vm)
	}
	return r, nil
}

// readResponses reads the per-client responses to one cc command from its
// miniccc_responses/<id> directory. byUUID names clients by VM; a client it
// does not know is keyed by its UUID. A command with no responses yet has no
// directory and gives an empty result.
func readResponses(dir string, byUUID map[string]string) (map[string]CCResult, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]CCResult{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("read responses: %w", err)
	}

	res := map[string]CCResult{}
	for _, e := range entries {
		r, err := readResponse(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("response %v: %w", e.Name(), err)
		}
		name := byUUID[e.Name()]
		if name == "" {
			name = e.Name()
		}
		res[name] = r
	}
	return res, nil
}

// readResponse reads one client's response directory. Missing files are
// empty output and noExitCode.
func readResponse(path string) (CCResult, error) {
	read := func(f string) (string, error) {
		b, err := os.ReadFile(filepath.Join(path, f))
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return string(b), err
	}

	r := CCResult{ExitCode: noExitCode}
	var code string
	var err error
	if r.Stdout, err = read("stdout"); err != nil {
		return r, err
	}
	if r.Stderr, err = read("stderr"); err != nil {
		return r, err
	}
	if code, err = read("exitcode"); err != nil {
		return r, err
	}
	if code = strings.TrimSpace(code); code != "" {
		if r.ExitCode, err = strconv.Atoi(code); err != nil {
			return r, fmt.Errorf("exit code %q: %w", code, err)
		}
	}
	return r, nil
}
