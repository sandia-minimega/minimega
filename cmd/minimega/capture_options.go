// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
)

// Kinds of pcap captures that take key=value options.
const (
	captureKindVM     = "vm"
	captureKindBridge = "bridge"
)

// captureOptionKinds lists the kinds that accept each option.
var captureOptionKinds = map[string][]string{
	"snaplen":   {captureKindVM, captureKindBridge},
	"filter":    {captureKindVM, captureKindBridge},
	"maxsize":   {captureKindVM, captureKindBridge},
	"direction": {captureKindVM},
}

var errFilenameOption = errors.New("give the file name before the options")

// checkCaptureFilename refuses a file name that looks like an option.
func checkCaptureFilename(fname string) error {
	if strings.Contains(fname, "=") {
		return errFilenameOption
	}

	return nil
}

// splitCaptureOptions splits key=value options into a map and checks that
// each key is known for kind and given only once.
func splitCaptureOptions(kind string, opts []string) (map[string]string, error) {
	res := map[string]string{}

	for _, opt := range opts {
		k, v, ok := strings.Cut(opt, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid option %q: use key=value", opt)
		}

		kinds, known := captureOptionKinds[k]
		if !known {
			return nil, fmt.Errorf("unknown option: %v", k)
		}

		var allowed bool
		for _, v := range kinds {
			allowed = allowed || v == kind
		}
		if !allowed {
			return nil, fmt.Errorf("option %v is not supported for %v captures", k, kind)
		}

		if _, dup := res[k]; dup {
			return nil, fmt.Errorf("option %v is given more than once", k)
		}

		res[k] = v
	}

	return res, nil
}

// parseCaptureOptions returns the config for a new pcap capture of kind: the
// namespace settings in base with the options on top.
func parseCaptureOptions(kind string, base bridge.CaptureConfig, opts []string) (bridge.CaptureConfig, error) {
	c := base

	m, err := splitCaptureOptions(kind, opts)
	if err != nil {
		return c, err
	}

	if v, ok := m["snaplen"]; ok {
		i, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return c, fmt.Errorf("invalid snaplen %q", v)
		}
		if err := bridge.ValidateSnapLen(uint32(i)); err != nil {
			return c, err
		}

		c.SnapLen = uint32(i)
	}

	if v, ok := m["filter"]; ok {
		c.Filter = v
	}

	if v, ok := m["maxsize"]; ok {
		size, err := bridge.ParseSize(v)
		if err != nil {
			return c, err
		}

		c.MaxSize = size
	}

	if v, ok := m["direction"]; ok {
		c.Direction = v
	}

	return c, c.Validate()
}
