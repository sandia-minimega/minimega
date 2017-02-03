// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bridge"
	"bytes"
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"
)

const (
	DisconnectedVLAN = -1
)

const (
	DefaultBridge = "mega_bridge"
	TapFmt        = "mega_tap%v"
	TapReapRate   = time.Second
)

type Tap struct {
	lan  int
	host bool
}

var bridges = bridge.NewBridges(DefaultBridge, TapFmt)

// tapReaperStart periodically calls bridges.ReapTaps
func tapReaperStart() {
	go func() {
		for {
			time.Sleep(TapReapRate)
			log.Debugln("periodic reapTaps")
			if err := bridges.ReapTaps(); err != nil {
				log.Errorln(err)
			}
		}
	}()
}

// destroy all bridges
func bridgesDestroy() error {
	if err := bridges.Destroy(); err != nil {
		return err
	}

	// Clean up bridges file
	bridgeFile := filepath.Join(*f_base, "bridges")
	if err := os.Remove(bridgeFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not remove bridge file: %v", err)
	}

	return nil
}

// getBridge is a wrapper for `bridges.Get` that gets the bridge and then
// updates the bridges state file on disk.
func getBridge(b string) (*bridge.Bridge, error) {
	br, err := bridges.Get(b)
	if err != nil {
		return nil, err
	}

	log.Debugln("updating bridge info")
	writeOrDie(filepath.Join(*f_base, "bridges"), bridgeInfo())

	return br, nil
}

// bridgeInfo returns formatted information about all the bridges.
func bridgeInfo() string {
	info := bridges.Info()
	if len(info) == 0 {
		return ""
	}

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "Bridge\tExisted before minimega\tActive VLANS\n")
	for _, i := range info {
		fmt.Fprintf(w, "%v\t%v\t%v\n", i.Name, i.PreExist, i.VLANs)
	}

	w.Flush()
	return o.String()
}

// hostTapCreate creates a host tap based on the supplied arguments.
func hostTapCreate(b, tap, v string) (string, error) {
	if b == "" {
		b = DefaultBridge
	}

	if isReserved(b) {
		return "", fmt.Errorf("`%s` is a reserved word -- cannot use for bridge name", b)
	}

	if isReserved(tap) {
		return "", fmt.Errorf("`%s` is a reserved word -- cannot use for tap name", tap)
	}

	vlan, err := lookupVLAN(v)
	if err != nil {
		return "", err
	}

	br, err := getBridge(b)
	if err != nil {
		return "", err
	}

	return br.CreateHostTap(tap, vlan)
}

// hostTapList populates resp with information about all the host taps.
func hostTapList(resp *minicli.Response) {
	resp.Header = []string{"bridge", "tap", "vlan"}
	resp.Tabular = [][]string{}

	// no namespace active => add an extra column
	ns := GetNamespace()
	if ns == nil {
		resp.Header = append(resp.Header, "namespace")
	}

	// find all the host taps first
	for _, tap := range bridges.HostTaps() {
		// skip taps that don't belong to the active namespace
		if ns != nil && !ns.HasTap(tap.Name) {
			continue
		}

		row := []string{
			tap.Bridge, tap.Name, printVLAN(tap.VLAN),
		}

		// no namespace active => find namespace tap belongs to so that we can
		// populate that column
		if ns == nil {
			v := ""
			for _, n := range ListNamespaces() {
				if ns := GetOrCreateNamespace(n); ns.HasTap(tap.Name) {
					v = ns.Name
					break
				}
			}

			row = append(row, v)
		}

		resp.Tabular = append(resp.Tabular, row)
	}
}

// hostTapDelete deletes a host tap by name or all host taps if Wildcard is
// specified.
func hostTapDelete(s string) error {
	ns := GetNamespace()

	delTap := func(t bridge.Tap) error {
		br, err := getBridge(t.Bridge)
		if err != nil {
			return err
		}

		if err := br.DestroyTap(t.Name); err != nil {
			return err
		}

		// update the host taps for the namespace
		if ns != nil {
			ns.RemoveTap(t.Name)
		}

		return nil
	}

	if s == Wildcard {
		for _, tap := range bridges.HostTaps() {
			// skip taps that don't belong to the active namespace
			if ns != nil && !ns.HasTap(tap.Name) {
				continue
			}

			if err := delTap(tap); err != nil {
				return err
			}
		}

		return nil
	}

	tap, err := bridges.FindTap(s)
	if err != nil {
		return err
	} else if !tap.Host {
		return errors.New("not a host tap")
	} else if namespace != "" && !namespaces[namespace].Taps[tap.Name] {
		return errors.New("not a host tap in active namespace")
	}

	return delTap(tap)
}
