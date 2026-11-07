// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// MaxMirrors is the most mirrors that Open vSwitch applies on one bridge. It
// accepts more Mirror rows without an error, but ignores them.
const MaxMirrors = 32

// Mirror kinds, written to external_ids:minimega-kind.
const (
	MirrorKindTap       = "tap-mirror"
	MirrorKindBridge    = "capture-bridge"
	MirrorKindNamespace = "capture-namespace"
	MirrorKindExternal  = "external"
)

// The external_ids keys that minimega sets on its mirrors and capture ports.
const (
	tagNamespace = "minimega-namespace"
	tagKind      = "minimega-kind"
	tagPID       = "minimega-pid"
	tagCapture   = "minimega-capture"
)

// MirrorTags say who owns a mirror.
type MirrorTags struct {
	Namespace string
	Kind      string
	Capture   string // capture ID, for namespace captures
}

// args returns the external_ids column values for an ovs-vsctl create or set.
func (t MirrorTags) args() []string {
	res := []string{
		fmt.Sprintf("external_ids:%v=%q", tagNamespace, t.Namespace),
		fmt.Sprintf("external_ids:%v=%q", tagKind, t.Kind),
		fmt.Sprintf("external_ids:%v=%q", tagPID, strconv.Itoa(os.Getpid())),
	}

	if t.Capture != "" {
		res = append(res, fmt.Sprintf("external_ids:%v=%q", tagCapture, t.Capture))
	}

	return res
}

// MirrorInfo describes one Mirror row of a bridge.
type MirrorInfo struct {
	Bridge    string
	Mirror    string
	Output    string
	Namespace string
	Kind      string
}

// ovsTable is a table that `ovs-vsctl --format=json list` prints.
type ovsTable struct {
	Headings []string            `json:"headings"`
	Data     [][]json.RawMessage `json:"data"`
}

// ovsList lists the columns of table.
func ovsList(table string, columns ...string) ([]map[string]json.RawMessage, error) {
	args := []string{"--format=json", "--columns=" + strings.Join(columns, ","), "list", table}

	out, err := ovsCmdWrapper(args)
	if err != nil {
		return nil, err
	}

	var t ovsTable
	if err := json.Unmarshal([]byte(out), &t); err != nil {
		return nil, fmt.Errorf("parse ovs-vsctl list %v: %v", table, err)
	}

	var res []map[string]json.RawMessage
	for _, row := range t.Data {
		m := map[string]json.RawMessage{}
		for i, v := range row {
			if i < len(t.Headings) {
				m[t.Headings[i]] = v
			}
		}
		res = append(res, m)
	}

	return res, nil
}

// ovsAtoms returns the atoms of an OVSDB value: a string, ["uuid", x], or
// ["set", [...]].
func ovsAtoms(v json.RawMessage) []string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return []string{s}
	}

	var pair []json.RawMessage
	if json.Unmarshal(v, &pair) != nil || len(pair) != 2 {
		return nil
	}

	var kind string
	json.Unmarshal(pair[0], &kind)

	switch kind {
	case "uuid", "named-uuid":
		var u string
		json.Unmarshal(pair[1], &u)
		return []string{u}
	case "set":
		var items []json.RawMessage
		json.Unmarshal(pair[1], &items)

		var res []string
		for _, item := range items {
			res = append(res, ovsAtoms(item)...)
		}
		return res
	}

	return nil
}

// ovsMap returns an OVSDB map value, ["map", [[k, v], ...]].
func ovsMap(v json.RawMessage) map[string]string {
	res := map[string]string{}

	var pair []json.RawMessage
	if json.Unmarshal(v, &pair) != nil || len(pair) != 2 {
		return res
	}

	var kvs [][]string
	json.Unmarshal(pair[1], &kvs)

	for _, kv := range kvs {
		if len(kv) == 2 {
			res[kv[0]] = kv[1]
		}
	}

	return res
}

func ovsString(v json.RawMessage) string {
	if a := ovsAtoms(v); len(a) > 0 {
		return a[0]
	}

	return ""
}

// ListMirrors lists the Mirror rows of the bridge named br, or of every
// bridge when br is empty, from OVSDB. It also lists mirrors that minimega did
// not make, with kind external.
func ListMirrors(br string) ([]MirrorInfo, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return listMirrors(br)
}

func listMirrors(br string) ([]MirrorInfo, error) {
	brs, err := ovsList("Bridge", "name", "mirrors")
	if err != nil {
		return nil, err
	}

	mirrors, err := ovsList("Mirror", "_uuid", "name", "output_port", "external_ids")
	if err != nil {
		return nil, err
	}

	ports, err := ovsList("Port", "_uuid", "name")
	if err != nil {
		return nil, err
	}

	portNames := map[string]string{}
	for _, p := range ports {
		portNames[ovsString(p["_uuid"])] = ovsString(p["name"])
	}

	byUUID := map[string]map[string]json.RawMessage{}
	for _, m := range mirrors {
		byUUID[ovsString(m["_uuid"])] = m
	}

	var res []MirrorInfo

	for _, b := range brs {
		name := ovsString(b["name"])
		if br != "" && name != br {
			continue
		}

		for _, u := range ovsAtoms(b["mirrors"]) {
			m, ok := byUUID[u]
			if !ok {
				continue
			}

			ids := ovsMap(m["external_ids"])
			kind := ids[tagKind]
			if kind == "" {
				kind = MirrorKindExternal
			}

			res = append(res, MirrorInfo{
				Bridge:    name,
				Mirror:    ovsString(m["name"]),
				Output:    portNames[ovsString(m["output_port"])],
				Namespace: ids[tagNamespace],
				Kind:      kind,
			})
		}
	}

	sort.Slice(res, func(i, j int) bool {
		if res[i].Bridge != res[j].Bridge {
			return res[i].Bridge < res[j].Bridge
		}
		return res[i].Mirror < res[j].Mirror
	})

	return res, nil
}

// ovsMirrorCount returns the number of mirrors on bridge.
func ovsMirrorCount(bridge string) (int, error) {
	out, err := ovsCmdWrapper([]string{"get", "Bridge", bridge, "mirrors"})
	if err != nil {
		return 0, err
	}

	out = strings.Trim(strings.TrimSpace(out), "[]")
	if out == "" {
		return 0, nil
	}

	return len(strings.Split(out, ",")), nil
}

// checkMirrorBudget fails when bridge already has MaxMirrors mirrors.
func checkMirrorBudget(bridge string) error {
	n, err := ovsMirrorCount(bridge)
	if err != nil {
		return err
	}

	if n >= MaxMirrors {
		host, _ := os.Hostname()
		return fmt.Errorf("bridge %v on %v has %v mirrors (Open vSwitch limit)", bridge, host, n)
	}

	return nil
}

// tagCapturePort tags a capture output port, so that a later minimega can
// remove it after a crash.
func tagCapturePort(tap string, tags MirrorTags) error {
	args := append([]string{"set", "Port", tap}, tags.args()...)

	_, err := ovsCmdWrapper(args)
	return err
}

// pidAlive reports whether a process with pid runs on this host.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if pid == os.Getpid() {
		return true
	}

	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// stale reports whether external_ids belong to a minimega process that no
// longer runs.
func stale(ids map[string]string) bool {
	v, ok := ids[tagPID]
	if !ok {
		return false
	}

	pid, err := strconv.Atoi(v)
	return err != nil || !pidAlive(pid)
}

// CleanStaleMirrors removes the mirrors and capture ports that a minimega
// process made before it crashed. Run it at start and in nuke.
func CleanStaleMirrors() error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	brs, err := ovsList("Bridge", "name", "mirrors", "ports")
	if err != nil {
		return err
	}

	mirrors, err := ovsList("Mirror", "_uuid", "name", "external_ids")
	if err != nil {
		return err
	}

	ports, err := ovsList("Port", "_uuid", "name", "external_ids")
	if err != nil {
		return err
	}

	staleMirrors := map[string]string{}
	for _, m := range mirrors {
		if stale(ovsMap(m["external_ids"])) {
			staleMirrors[ovsString(m["_uuid"])] = ovsString(m["name"])
		}
	}

	stalePorts := map[string]string{}
	for _, p := range ports {
		if stale(ovsMap(p["external_ids"])) {
			stalePorts[ovsString(p["_uuid"])] = ovsString(p["name"])
		}
	}

	var errs []error

	for _, b := range brs {
		name := ovsString(b["name"])

		for _, u := range ovsAtoms(b["mirrors"]) {
			if m, ok := staleMirrors[u]; ok {
				log.Info("removing stale mirror %v on %v", m, name)
				if _, err := ovsCmdWrapper([]string{"remove", "Bridge", name, "mirrors", u}); err != nil {
					errs = append(errs, err)
				}
			}
		}

		for _, u := range ovsAtoms(b["ports"]) {
			if p, ok := stalePorts[u]; ok {
				log.Info("removing stale capture port %v on %v", p, name)
				if err := ovsDelPort(name, p); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	return errors.Join(errs...)
}
