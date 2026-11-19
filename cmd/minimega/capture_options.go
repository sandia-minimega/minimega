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
	"github.com/sandia-minimega/minimega/v2/internal/vlans"
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
	"format":    {captureKindVM, captureKindBridge},
	"ports":     {captureKindBridge},
	"vlans":     {captureKindBridge},
	"vms":       {captureKindBridge},
}

// nsCaptureOptionKeys lists the options of namespace captures that
// captureOptionKinds does not have.
var nsCaptureOptionKeys = map[string]bool{"bridges": true, "attribute": true, "names": true, "window": true}

var errFilenameOption = errors.New("give the file name before the options")

// checkCaptureFilename refuses a file name that starts with a known option
// key and "=".
func checkCaptureFilename(fname string) error {
	k, _, ok := strings.Cut(fname, "=")
	if _, known := captureOptionKinds[k]; ok && (known || nsCaptureOptionKeys[k]) {
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

		if v == "" && k != "filter" {
			return nil, fmt.Errorf("option %v needs a value", k)
		}

		res[k] = v
	}

	return res, nil
}

// parseCaptureVLANs resolves a comma-separated list of VLAN aliases and IDs in
// namespace. It never allocates a VLAN.
func parseCaptureVLANs(namespace, s string) ([]int, error) {
	var res []int

	for _, v := range strings.Split(s, ",") {
		if v == "" {
			continue
		}

		if id, err := strconv.Atoi(v); err == nil {
			if id < 1 || id > 4095 {
				return nil, fmt.Errorf("invalid VLAN %v: use 1 to 4095", id)
			}
			res = append(res, id)
			continue
		}

		id, err := vlans.GetVLAN(namespace, v)
		if err != nil {
			return nil, fmt.Errorf("unknown VLAN %v in namespace %v", v, namespace)
		}
		res = append(res, id)
	}

	if len(res) == 0 {
		return nil, errors.New("vlans= needs one or more VLANs")
	}

	return res, nil
}

// parseCaptureOptions returns the config for a new pcap capture of kind in
// namespace: the namespace settings in base with the options on top.
func parseCaptureOptions(kind, namespace string, base bridge.CaptureConfig, opts []string) (bridge.CaptureConfig, error) {
	c := base
	c.Namespace = namespace

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

	if v, ok := m["format"]; ok {
		c.Format = v
	}

	if v, ok := m["ports"]; ok {
		c.Ports = v
	}

	if v, ok := m["vlans"]; ok {
		if c.VLANs, err = parseCaptureVLANs(namespace, v); err != nil {
			return c, err
		}
	}

	if v, ok := m["vms"]; ok {
		if c.VMs, err = splitCaptureNames("vms", v); err != nil {
			return c, err
		}
	}

	return c, c.Validate()
}

// splitCaptureNames splits the comma-separated names of option key and
// refuses an empty list.
func splitCaptureNames(key, s string) ([]string, error) {
	var res []string

	for _, v := range strings.Split(s, ",") {
		if v != "" {
			res = append(res, v)
		}
	}

	if len(res) == 0 {
		return nil, fmt.Errorf("%v= needs one or more names", key)
	}

	return res, nil
}

// checkCaptureVMs checks that each name in vms is the name of a VM in ns.
func checkCaptureVMs(ns *Namespace, vms []string) error {
	for _, name := range vms {
		if vm := ns.FindVM(name); vm == nil || vm.GetName() != name {
			return fmt.Errorf("unknown VM %v in namespace %v", name, ns.Name)
		}
	}

	return nil
}
