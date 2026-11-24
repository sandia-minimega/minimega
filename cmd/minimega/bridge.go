// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/vlans"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	DisconnectedVLAN = -1
)

const (
	DefaultBridge = "mega_bridge"
	TapFmt        = "mega_tap%v"
	BondFmt       = "mega_bond%v"
	TapReapRate   = time.Second
)

type Tap struct {
	lan  int
	host bool
}

var bridges = bridge.NewBridges(DefaultBridge, TapFmt, BondFmt)

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
	return bridgesFile.reset(func() error {
		bridgeFile := filepath.Join(*f_base, "bridges")
		if err := os.Remove(bridgeFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not remove bridge file: %v", err)
		}
		return nil
	})
}

// bridgeFileDelay is the maximum time that a change other than a new bridge
// waits before bridgeFileWriter writes it.
const bridgeFileDelay = time.Second

// bridgeFileWriter writes the bridges state file, which nuke reads to find the
// bridges to delete. It writes the file at once for a bridge that the last
// write did not include. It writes other changes, such as VLANs, at most once
// each delay.
type bridgeFileWriter struct {
	mu    sync.Mutex
	delay time.Duration
	// write writes the file and returns the names of the bridges in it.
	write   func() map[string]bool
	names   map[string]bool
	pending bool
}

var bridgesFile = &bridgeFileWriter{
	delay: bridgeFileDelay,
	write: func() map[string]bool {
		info := bridges.Info()
		mustWrite(filepath.Join(*f_base, "bridges"), formatBridgeInfo(info))

		names := map[string]bool{}
		for _, i := range info {
			names[i.Name] = true
		}
		return names
	},
}

// update records a use of bridge name.
func (w *bridgeFileWriter) update(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.names[name] {
		w.pending = false
		w.names = w.write()
		return
	}

	if !w.pending {
		w.pending = true
		time.AfterFunc(w.delay, w.flush)
	}
}

func (w *bridgeFileWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.pending {
		w.pending = false
		w.names = w.write()
	}
}

// reset cancels a pending write, forgets the bridges, and runs fn.
func (w *bridgeFileWriter) reset(fn func() error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending = false
	w.names = nil

	return fn()
}

// getBridge is a wrapper for `bridges.Get` that gets the bridge and then
// updates the bridges state file on disk.
func getBridge(b string) (*bridge.Bridge, error) {
	br, err := bridges.Get(b)
	if err != nil {
		return nil, err
	}

	bridgesFile.update(br.Name)

	return br, nil
}

// formatBridgeInfo returns formatted information about the bridges in info.
func formatBridgeInfo(info []bridge.BridgeInfo) string {
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
func hostTapCreate(ns, b, tap string, v int) (string, error) {
	if b == "" {
		b = DefaultBridge
	}

	if isReserved(b) {
		return "", fmt.Errorf("`%s` is a reserved word -- cannot use for bridge name", b)
	}

	if isReserved(tap) {
		return "", fmt.Errorf("`%s` is a reserved word -- cannot use for tap name", tap)
	}

	br, err := getBridge(b)
	if err != nil {
		return "", err
	}

	tap, err = br.CreateHostTap(tap, v)
	if err == nil {
		br.SetTapOwner(tap, ns, "")
		mustWrite(filepath.Join(*f_base, "taps"), hostTapInfo())
	}

	return tap, err
}

// hostTapList populates resp with information about all the host taps.
func hostTapList(ns *Namespace, resp *minicli.Response) {
	resp.Header = []string{"bridge", "tap", "vlan"}
	resp.Tabular = [][]string{}

	// find all the host taps first
	for _, tap := range bridges.HostTaps() {
		if !ns.Taps[tap.Name] {
			continue
		}

		row := []string{
			tap.Bridge, tap.Name, printVLAN(ns.Name, tap.VLAN),
		}

		resp.Tabular = append(resp.Tabular, row)
	}
}

// hostTapDelete deletes a host tap by name or all host taps if Wildcard is
// specified.
func hostTapDelete(ns *Namespace, s string) error {
	// helper to find and delete a tap t
	delTap := func(t string) error {
		tap, err := bridges.FindTap(t)
		if err != nil {
			return err
		} else if !tap.Host {
			return errors.New("not a host tap")
		} else if !ns.Taps[tap.Name] {
			return errors.New("not a host tap in active namespace")
		}

		br, err := getBridge(tap.Bridge)
		if err != nil {
			return err
		}

		if err := br.DestroyTap(tap.Name); err != nil {
			return err
		}

		// update the host taps for the namespace
		delete(ns.Taps, tap.Name)
		return nil
	}

	if s == Wildcard {
		for tap := range ns.Taps {
			if err := delTap(tap); err != nil {
				return err
			}
		}

		return nil
	}

	err := delTap(s)
	if err == nil {
		mustWrite(filepath.Join(*f_base, "taps"), hostTapInfo())
	}

	return err
}

func recoverHostTaps() error {
	f, err := os.Open(filepath.Join(*f_base, "taps"))
	if err == nil {
		var (
			scanner = bufio.NewScanner(f)
			skip    = true
		)

		for scanner.Scan() {
			if skip {
				// skip first line in file (header data)
				skip = false
				continue
			}

			r, err := parseHostTap(scanner.Text())
			if err != nil {
				return err
			}

			br, err := bridges.Get(r.bridge)
			if err != nil {
				return fmt.Errorf("unable to get bridge %s for host tap %s: %w", r.bridge, r.tap, err)
			}

			vlan, err := vlans.GetVLAN("", r.alias)
			if err != nil {
				return fmt.Errorf("unable to get VLAN ID for alias %s on host tap %s: %w", r.alias, r.tap, err)
			}

			br.RecoverTap(r.tap, "", vlan, true)
			if r.namespace != "" {
				br.SetTapOwner(r.tap, r.namespace, "")
			}
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("unable to process taps file: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("unable to open taps file: %w", err)
	}

	return nil
}

// cleanKeep returns the mirror kinds that the clean-up at start keeps. With
// -recover, it keeps the tap mirrors, which recoverMirrors takes back.
func cleanKeep(recovering bool) []string {
	if recovering {
		return []string{bridge.MirrorKindTap}
	}

	return nil
}

// recoverMirrors takes back the tap mirrors of the recovered taps and adds
// them to the mirrors of their namespaces. Run it after the taps are
// recovered.
func recoverMirrors() {
	infos, err := bridges.RecoverMirrors()
	if err != nil {
		log.Warn("unable to recover tap mirrors: %v", err)
	}

	adoptMirrors(infos, GetOrCreateNamespace)
}

// adoptMirrors adds the recovered mirrors to the mirrors of the namespaces
// that get returns.
func adoptMirrors(infos []bridge.MirrorInfo, get func(string) *Namespace) {
	for _, m := range infos {
		if m.Namespace != "" {
			get(m.Namespace).Mirrors[m.Output] = true
		}
	}
}

// hostTapRecord is one line of the host taps recovery file.
type hostTapRecord struct {
	bridge, tap, alias, namespace string
}

// parseHostTap parses one line of the host taps recovery file. A line without
// the namespace column takes the namespace from the VLAN alias.
func parseHostTap(line string) (hostTapRecord, error) {
	fields := strings.Fields(line)

	switch len(fields) {
	case 3:
		a := vlans.ParseAlias("", fields[2])
		return hostTapRecord{fields[0], fields[1], fields[2], a.Namespace}, nil
	case 4:
		return hostTapRecord{fields[0], fields[1], fields[2], fields[3]}, nil
	}

	return hostTapRecord{}, fmt.Errorf("expected three or four columns in taps file: got %d", len(fields))
}

// hostTapInfo returns formatted information about all the host taps.
func hostTapInfo() string {
	return formatHostTaps(bridges.HostTaps())
}

// formatHostTaps returns the host taps recovery file contents for taps.
func formatHostTaps(taps []bridge.Tap) string {
	if len(taps) == 0 {
		return ""
	}

	var o bytes.Buffer

	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)

	fmt.Fprintf(w, "Bridge\tTap\tVLAN\tNamespace\n")

	for _, t := range taps {
		alias := strings.Fields(vlans.PrintVLAN("", t.VLAN))[0]
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", t.Bridge, t.Name, alias, t.Namespace)
	}

	w.Flush()
	return o.String()
}

func mirrorDelete(ns *Namespace, name string) error {
	delMirror := func(m string) error {
		if !ns.Mirrors[m] {
			return errors.New("not a valid mirror")
		}

		tap, err := bridges.FindTap(m)
		if err != nil {
			return err
		}

		br, err := getBridge(tap.Bridge)
		if err != nil {
			return err
		}

		if err := br.DestroyMirror(m); err != nil {
			return err
		}

		// update the mirrors for the namespace
		delete(ns.Mirrors, m)
		return nil
	}

	if name == Wildcard || name == "" {
		// try every mirror, so that one error does not keep the others
		var errs []error
		for mirror := range ns.Mirrors {
			if err := delMirror(mirror); err != nil {
				errs = append(errs, fmt.Errorf("%v: %w", mirror, err))
			}
		}

		return errors.Join(errs...)
	}

	return delMirror(name)
}

// mirrorDeleteVM looks up the name of the interface(s) for the VM and then
// calls mirrorDelete on them.
func mirrorDeleteVM(ns *Namespace, svm, si string) error {
	vm := ns.FindVM(svm)
	if vm == nil {
		return vmNotFound(svm)
	}

	// delete all mirrors for all interfaces
	if si == Wildcard {
		networks := vm.GetNetworks()

		for _, nic := range networks {
			if !ns.Mirrors[nic.Tap] {
				continue
			}

			if err := mirrorDelete(ns, nic.Tap); err != nil {
				return err
			}
		}

		return nil
	}

	i, err := strconv.Atoi(si)
	if err != nil {
		return fmt.Errorf("invalid interface number: `%v`", si)
	}

	nic, err := vm.GetNetwork(i)
	if err != nil {
		return err
	}

	return mirrorDelete(ns, nic.Tap)
}
