// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// fakeOVSDB answers ovs-vsctl list and get commands from fixed JSON tables and
// records the other commands.
func fakeOVSDB(t *testing.T, tables map[string]string, mirrorCount int) *fakeCommands {
	t.Helper()

	f := useFakeCommands(t)
	ovsExec = func(args []string) (string, error) {
		f.add("ovs-vsctl " + strings.Join(args, " "))

		if len(args) == 4 && args[2] == "list" {
			return tables[args[3]], nil
		}
		if len(args) == 4 && args[0] == "get" && args[3] == "mirrors" {
			var uuids []string
			for i := 0; i < mirrorCount; i++ {
				uuids = append(uuids, fmt.Sprintf("%08d-0000-0000-0000-000000000000", i))
			}
			return "[" + strings.Join(uuids, ", ") + "]\n", nil
		}

		return "", nil
	}

	return f
}

func ovsdbTables(pid int) map[string]string {
	return map[string]string{
		"Bridge": `{"data":[["mega_bridge",["set",[["uuid","m1"],["uuid","m2"],["uuid","m3"]]],["set",[["uuid","p1"],["uuid","p2"]]]],` +
			`["phenix",["uuid","m4"],["uuid","p3"]]],"headings":["name","mirrors","ports"]}`,
		"Mirror": fmt.Sprintf(`{"data":[`+
			`[["uuid","m1"],"mirror-mega_tap1",["uuid","p1"],["map",[["minimega-kind","capture-bridge"],["minimega-namespace","exp"],["minimega-pid","%d"]]]],`+
			`[["uuid","m2"],"mirror-mega_tap2",["uuid","p2"],["map",[["minimega-kind","tap-mirror"],["minimega-namespace","exp"],["minimega-pid","999999999"]]]],`+
			`[["uuid","m3"],"other",["set",[]],["map",[]]],`+
			`[["uuid","m4"],"erspan",["uuid","p3"],["map",[["minimega-kind","app-erspan"],["minimega-namespace","exp2"]]]]`+
			`],"headings":["_uuid","name","output_port","external_ids"]}`, pid),
		"Port": `{"data":[` +
			`[["uuid","p1"],"mega_tap1",["map",[]]],` +
			`[["uuid","p2"],"mega_tap2",["map",[["minimega-pid","999999999"]]]],` +
			`[["uuid","p3"],"eth9",["map",[]]]` +
			`],"headings":["_uuid","name","external_ids"]}`,
	}
}

func TestListMirrors(t *testing.T) {
	fakeOVSDB(t, ovsdbTables(os.Getpid()), 0)

	got, err := ListMirrors("")
	if err != nil {
		t.Fatal(err)
	}

	want := []MirrorInfo{
		{"mega_bridge", "mirror-mega_tap1", "mega_tap1", "exp", "capture-bridge"},
		{"mega_bridge", "mirror-mega_tap2", "mega_tap2", "exp", "tap-mirror"},
		{"mega_bridge", "other", "", "", "external"},
		{"phenix", "erspan", "eth9", "exp2", "app-erspan"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListMirrors() =\n%+v\nwant\n%+v", got, want)
	}

	got, err = ListMirrors("phenix")
	if err != nil || len(got) != 1 {
		t.Errorf("ListMirrors(phenix) = %v %v", got, err)
	}
}

// ownerMap returns the OVSDB map of the owner tags of o and kind.
func ownerMap(o owner, kind string) string {
	return fmt.Sprintf(`["map",[["minimega-boot-id",%q],["minimega-kind",%q],["minimega-pid",%q],["minimega-pid-ns",%q],["minimega-start-time",%q]]]`,
		o.boot, kind, strconv.Itoa(o.pid), o.pidNS, o.start)
}

// staleTables returns OVSDB tables with mirrors of live, dead and unknown
// owners. The mirrors that end in "stale" are stale.
func staleTables(t *testing.T) map[string]string {
	self, err := selfOwner()
	if err != nil {
		t.Fatal(err)
	}

	dead := self
	dead.pid = 999999999

	otherNS := dead
	otherNS.pidNS = "1"

	oldBoot := self
	oldBoot.boot = "00000000-0000-0000-0000-000000000000"

	reused := self
	reused.start = "1"

	rows := []struct{ name, ids string }{
		{"live", ownerMap(self, "capture-bridge")},
		{"dead-stale", ownerMap(dead, "capture-bridge")},
		{"dead-tap-stale", ownerMap(dead, "tap-mirror")},
		{"other-ns", ownerMap(otherNS, "capture-bridge")},
		{"old-boot-stale", ownerMap(oldBoot, "capture-bridge")},
		{"reused-pid-stale", ownerMap(reused, "capture-bridge")},
		{"pid-only", `["map",[["minimega-kind","capture-bridge"],["minimega-pid","999999999"]]]`},
		{"untagged", `["map",[]]`},
	}

	var uuids, mirrors []string
	for _, r := range rows {
		uuids = append(uuids, fmt.Sprintf(`["uuid",%q]`, r.name))
		mirrors = append(mirrors, fmt.Sprintf(`[["uuid",%q],%q,%s]`, r.name, r.name, r.ids))
	}

	return map[string]string{
		"Bridge": `{"data":[["mega_bridge",["set",[` + strings.Join(uuids, ",") + `]],["set",[["uuid","p1"],["uuid","p2"]]]]],` +
			`"headings":["name","mirrors","ports"]}`,
		"Mirror": `{"data":[` + strings.Join(mirrors, ",") + `],"headings":["_uuid","name","external_ids"]}`,
		"Port": `{"data":[` +
			`[["uuid","p1"],"mega_tap1",` + ownerMap(self, "capture-bridge") + `],` +
			`[["uuid","p2"],"mega_tap2",` + ownerMap(dead, "capture-bridge") + `]` +
			`],"headings":["_uuid","name","external_ids"]}`,
	}
}

func TestCleanStaleMirrors(t *testing.T) {
	cmds := fakeOVSDB(t, staleTables(t), 0)

	if err := CleanStaleMirrors(); err != nil {
		t.Fatal(err)
	}

	for _, m := range []string{"live", "dead-stale", "dead-tap-stale", "other-ns", "old-boot-stale", "reused-pid-stale", "pid-only", "untagged"} {
		want := strings.HasSuffix(m, "-stale")
		if got := cmds.has("remove Bridge mega_bridge mirrors " + m); got != want {
			t.Errorf("mirror %v removed: %v, want %v", m, got, want)
		}
	}

	if !cmds.has("del-port mega_bridge mega_tap2") || cmds.has("mega_tap1") {
		t.Errorf("wrong capture ports removed: %q", cmds.cmds)
	}
}

// TestCleanStaleMirrorsKeep checks that the kinds in keep stay, such as the tap
// mirrors of the VMs that -recover recovers.
func TestCleanStaleMirrorsKeep(t *testing.T) {
	cmds := fakeOVSDB(t, staleTables(t), 0)

	if err := CleanStaleMirrors(MirrorKindTap); err != nil {
		t.Fatal(err)
	}

	if !cmds.has("remove Bridge mega_bridge mirrors dead-stale") || cmds.has("remove Bridge mega_bridge mirrors dead-tap-stale") {
		t.Errorf("tap mirror removed or capture mirror kept: %q", cmds.cmds)
	}
}

func TestMirrorBudget(t *testing.T) {
	cmds := fakeOVSDB(t, nil, MaxMirrors)

	b := testBridge()
	b.taps["tap1"] = &Tap{Name: "tap1"}

	err := b.CreateMirror("tap0", "tap1", "exp")
	if err == nil || !strings.Contains(err.Error(), "has 32 mirrors (Open vSwitch limit)") {
		t.Errorf("CreateMirror at the limit: %v", err)
	}

	cmds = fakeOVSDB(t, nil, MaxMirrors-1)
	if err := b.CreateMirror("tap0", "tap1", "exp"); err != nil {
		t.Fatal(err)
	}

	self, err := selfOwner()
	if err != nil {
		t.Fatal(err)
	}
	pid := fmt.Sprintf(`external_ids:minimega-pid="%d"`, os.Getpid())
	boot := fmt.Sprintf(`external_ids:minimega-boot-id=%q`, self.boot)
	start := fmt.Sprintf(`external_ids:minimega-start-time=%q`, self.start)
	pidNS := fmt.Sprintf(`external_ids:minimega-pid-ns=%q`, self.pidNS)
	for _, want := range []string{`external_ids:minimega-namespace="exp"`, `external_ids:minimega-kind="tap-mirror"`, pid, boot, start, pidNS} {
		if !cmds.has(want) {
			t.Errorf("mirror without %v: %q", want, cmds.cmds)
		}
	}
}

// TestStaleProcErrors checks that only a process that is gone makes a row
// stale: a process that this process cannot read or see, for example with
// hidepid, is live.
func TestStaleProcErrors(t *testing.T) {
	oldStat, oldKill := procStat, procKill
	t.Cleanup(func() { procStat, procKill = oldStat, oldKill })

	self := owner{boot: "boot", pidNS: "1", pid: 1, start: "5"}
	ids := map[string]string{tagBoot: "boot", tagPIDNS: "1", tagPID: "42", tagStart: "5"}

	stat := func(start string) []byte {
		return []byte("42 (minimega) " + strings.Repeat("0 ", 19) + start)
	}
	pathErr := func(err error) error {
		return &fs.PathError{Op: "open", Path: "/proc/42/stat", Err: err}
	}

	for name, tc := range map[string]struct {
		data []byte
		err  error
		kill error
		want bool
	}{
		"live":           {data: stat("5")},
		"reused":         {data: stat("6"), want: true},
		"gone":           {err: pathErr(syscall.ENOENT), kill: syscall.ESRCH, want: true},
		"hidden":         {err: pathErr(syscall.ENOENT), kill: syscall.EPERM},
		"hidden, signal": {err: pathErr(syscall.ENOENT)},
		"exited":         {err: pathErr(syscall.ESRCH), want: true},
		"hidepid":        {err: pathErr(syscall.EACCES)},
		"no perm":        {err: pathErr(syscall.EPERM)},
		"too long":       {err: pathErr(syscall.EIO)},
	} {
		procStat = func(int) ([]byte, error) { return tc.data, tc.err }
		procKill = func(pid int) error {
			if pid != 42 {
				t.Errorf("%v: signal to PID %v", name, pid)
			}
			return tc.kill
		}

		if got := stale(ids, self); got != tc.want {
			t.Errorf("%v: stale = %v, want %v", name, got, tc.want)
		}
	}
}

// tapMirrorMap returns the OVSDB map of a tap mirror of namespace exp that o
// made.
func tapMirrorMap(o owner) string {
	return fmt.Sprintf(`["map",[["minimega-boot-id",%q],["minimega-kind","tap-mirror"],["minimega-namespace","exp"],["minimega-pid",%q],["minimega-pid-ns",%q],["minimega-start-time",%q]]]`,
		o.boot, strconv.Itoa(o.pid), o.pidNS, o.start)
}

// TestRecoverMirrors checks that -recover takes back the tap mirror of a
// recovered tap, so that mirror delete removes it and a new mirror to the tap
// does not add a second row, and removes the tap mirror of another tap.
func TestRecoverMirrors(t *testing.T) {
	self, err := selfOwner()
	if err != nil {
		t.Fatal(err)
	}

	dead := self
	dead.pid = 999999999

	tables := map[string]string{
		"Bridge": `{"data":[["mega_bridge",["set",[["uuid","m1"],["uuid","m2"],["uuid","m3"],["uuid","m4"]]]]],"headings":["name","mirrors"]}`,
		"Mirror": `{"data":[` +
			`[["uuid","m1"],"mirror-mega_tap2",["uuid","p2"],` + tapMirrorMap(dead) + `],` +
			`[["uuid","m2"],"mirror-mega_tap9",["uuid","p9"],` + tapMirrorMap(dead) + `],` +
			`[["uuid","m3"],"mirror-mega_tap3",["uuid","p3"],` + tapMirrorMap(self) + `],` +
			`[["uuid","m4"],"mirror-mega_tap4",["uuid","p4"],` + ownerMap(dead, "capture-bridge") + `]` +
			`],"headings":["_uuid","name","output_port","external_ids"]}`,
		"Port": `{"data":[[["uuid","p2"],"mega_tap2"],[["uuid","p9"],"mega_tap9"],[["uuid","p3"],"mega_tap3"],[["uuid","p4"],"mega_tap4"]],` +
			`"headings":["_uuid","name"]}`,
	}
	cmds := fakeOVSDB(t, tables, 0)

	b := testBridge()
	b.Name = "mega_bridge"
	for _, tap := range []string{"mega_tap2", "mega_tap3", "mega_tap4"} {
		b.taps[tap] = &Tap{Name: tap, Bridge: b.Name, Host: true}
	}
	brs := Bridges{bridges: map[string]*Bridge{b.Name: b}}

	got, err := brs.RecoverMirrors()
	if err != nil {
		t.Fatal(err)
	}

	want := []MirrorInfo{{"mega_bridge", "mirror-mega_tap2", "mega_tap2", "exp", MirrorKindTap}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RecoverMirrors() = %+v, want %+v", got, want)
	}

	if !cmds.has(fmt.Sprintf(`set Mirror m1 external_ids:minimega-namespace="exp" external_ids:minimega-kind="tap-mirror" external_ids:minimega-boot-id=%q`, self.boot)) {
		t.Errorf("mirror m1 does not get the owner tags of this process: %q", cmds.cmds)
	}
	if !cmds.has("remove Bridge mega_bridge mirrors m2") {
		t.Errorf("mirror m2 of a tap that is not recovered stays: %q", cmds.cmds)
	}
	for _, m := range []string{"m1", "m3", "m4"} {
		if cmds.has("remove Bridge mega_bridge mirrors " + m) {
			t.Errorf("mirror %v removed: %q", m, cmds.cmds)
		}
	}
	if cmds.has("set Mirror m3") || cmds.has("set Mirror m4") {
		t.Errorf("a live or capture mirror got new tags: %q", cmds.cmds)
	}

	if err := b.CreateMirror("tap0", "mega_tap2", "exp"); err == nil || !strings.Contains(err.Error(), "already a mirror") {
		t.Errorf("second mirror to a recovered mirror tap: %v", err)
	}

	if err := b.DestroyMirror("mega_tap2"); err != nil {
		t.Errorf("delete of the recovered mirror: %v", err)
	}
	if !cmds.has("get mirror mirror-mega_tap2 -- remove bridge mega_bridge mirrors @m") {
		t.Errorf("recovered mirror not removed from Open vSwitch: %q", cmds.cmds)
	}
}
