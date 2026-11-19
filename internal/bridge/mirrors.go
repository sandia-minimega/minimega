// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
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
// The boot ID, PID namespace, PID and start time identify the minimega
// process that made a row.
const (
	tagNamespace = "minimega-namespace"
	tagKind      = "minimega-kind"
	tagBoot      = "minimega-boot-id"
	tagPIDNS     = "minimega-pid-ns"
	tagPID       = "minimega-pid"
	tagStart     = "minimega-start-time"
	tagCapture   = "minimega-capture"
)

// owner identifies a process on a host.
type owner struct {
	boot  string // boot ID of the kernel
	pidNS string // inode of the PID namespace
	pid   int
	start string // start time in clock ticks after boot
}

// selfOwner returns the owner of this process.
var selfOwner = sync.OnceValues(func() (owner, error) {
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return owner{}, err
	}

	// the link is "pid:[<inode>]"
	link, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return owner{}, err
	}
	pidNS := strings.TrimSuffix(strings.TrimPrefix(link, "pid:["), "]")
	if _, err := strconv.ParseUint(pidNS, 10, 64); err != nil {
		return owner{}, fmt.Errorf("unexpected PID namespace %q", link)
	}

	pid := os.Getpid()
	start, err := procStart(pid)
	if err != nil {
		return owner{}, err
	}

	return owner{strings.TrimSpace(string(boot)), pidNS, pid, start}, nil
})

// procStat reads /proc/<pid>/stat. Tests replace it.
var procStat = func(pid int) ([]byte, error) {
	return os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
}

// procKill sends signal 0 to process pid. Tests replace it.
var procKill = func(pid int) error {
	return syscall.Kill(pid, 0)
}

// procStart returns the start time of process pid, in clock ticks after
// boot, from /proc/<pid>/stat.
func procStart(pid int) (string, error) {
	data, err := procStat(pid)
	if err != nil {
		return "", err
	}

	// the fields after the command name start with field 3; starttime is 22
	s := string(data)
	fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
	if len(fields) < 20 {
		return "", fmt.Errorf("unexpected /proc/%d/stat", pid)
	}

	return fields[19], nil
}

// MirrorTags say who owns a mirror.
type MirrorTags struct {
	Namespace string
	Kind      string
	Capture   string // capture ID, for namespace captures
}

// args returns the external_ids column values for an ovs-vsctl create or set.
// When this process is unknown, it leaves out the owner, and CleanStaleMirrors
// returns the error.
func (t MirrorTags) args() []string {
	res := []string{
		fmt.Sprintf("external_ids:%v=%q", tagNamespace, t.Namespace),
		fmt.Sprintf("external_ids:%v=%q", tagKind, t.Kind),
	}

	if o, err := selfOwner(); err == nil {
		res = append(res,
			fmt.Sprintf("external_ids:%v=%q", tagBoot, o.boot),
			fmt.Sprintf("external_ids:%v=%q", tagPIDNS, o.pidNS),
			fmt.Sprintf("external_ids:%v=%q", tagPID, strconv.Itoa(o.pid)),
			fmt.Sprintf("external_ids:%v=%q", tagStart, o.start),
		)
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

// stale reports whether external_ids belong to a minimega process that no
// longer runs: a process of an earlier boot, or a process of the PID
// namespace of self that is gone or has another start time. A process of
// another PID namespace, such as another container, cannot be checked, so its
// rows are never stale. Rows without the boot ID are not checked either, and
// a process that this process cannot read or see, for example with hidepid,
// is live while signal 0 to its PID does not fail with ESRCH.
func stale(ids map[string]string, self owner) bool {
	boot, ok := ids[tagBoot]
	if !ok {
		return false
	}
	if boot != self.boot {
		return true
	}
	if ids[tagPIDNS] != self.pidNS {
		return false
	}

	pid, err := strconv.Atoi(ids[tagPID])
	if err != nil || pid <= 0 {
		return true
	}

	start, err := procStart(pid)
	if errors.Is(err, fs.ErrNotExist) {
		// hidepid=2 hides the live processes of other users
		return errors.Is(procKill(pid), syscall.ESRCH)
	}
	if err != nil {
		return errors.Is(err, syscall.ESRCH)
	}

	return start != ids[tagStart]
}

// CleanStaleMirrors removes the mirrors and capture ports that a minimega
// process made before it crashed, except the mirrors of the kinds in keep.
// Run it at start and in nuke.
func CleanStaleMirrors(keep ...string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	self, err := selfOwner()
	if err != nil {
		return fmt.Errorf("unable to identify this process: %v", err)
	}

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
		ids := ovsMap(m["external_ids"])
		if slices.Contains(keep, ids[tagKind]) {
			continue
		}

		if stale(ids, self) {
			staleMirrors[ovsString(m["_uuid"])] = ovsString(m["name"])
		}
	}

	stalePorts := map[string]string{}
	for _, p := range ports {
		if stale(ovsMap(p["external_ids"]), self) {
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

// RecoverMirrors takes back the tap mirrors that a minimega process made
// before it crashed, after -recover recovered the taps. A stale tap mirror
// to a recovered tap gets the owner tags of this process and is returned. A
// stale tap mirror to another tap is removed.
func (b Bridges) RecoverMirrors() ([]MirrorInfo, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	self, err := selfOwner()
	if err != nil {
		return nil, fmt.Errorf("unable to identify this process: %v", err)
	}

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
	var errs []error

	for _, row := range brs {
		name := ovsString(row["name"])
		br := b.bridges[name]

		for _, u := range ovsAtoms(row["mirrors"]) {
			m, ok := byUUID[u]
			if !ok {
				continue
			}

			ids := ovsMap(m["external_ids"])
			if ids[tagKind] != MirrorKindTap || !stale(ids, self) {
				continue
			}

			mirror := ovsString(m["name"])
			out := portNames[ovsString(m["output_port"])]

			if br != nil && br.taps[out] != nil && !br.taps[out].Defunct && !br.mirrors[out] {
				tags := MirrorTags{Namespace: ids[tagNamespace], Kind: MirrorKindTap}
				if _, err := ovsCmdWrapper(append([]string{"set", "Mirror", u}, tags.args()...)); err != nil {
					errs = append(errs, err)
					continue
				}

				log.Info("recovered mirror %v on %v", mirror, name)
				br.mirrors[out] = true
				res = append(res, MirrorInfo{Bridge: name, Mirror: mirror, Output: out, Namespace: tags.Namespace, Kind: MirrorKindTap})
				continue
			}

			log.Info("removing stale mirror %v on %v", mirror, name)
			if _, err := ovsCmdWrapper([]string{"remove", "Bridge", name, "mirrors", u}); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return res, errors.Join(errs...)
}
