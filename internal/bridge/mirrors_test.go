// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"
	"os"
	"reflect"
	"strings"
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

func TestCleanStaleMirrors(t *testing.T) {
	cmds := fakeOVSDB(t, ovsdbTables(os.Getpid()), 0)

	if err := CleanStaleMirrors(); err != nil {
		t.Fatal(err)
	}

	if !cmds.has("remove Bridge mega_bridge mirrors m2") || !cmds.has("del-port mega_bridge mega_tap2") {
		t.Errorf("stale rows not removed: %q", cmds.cmds)
	}
	if cmds.has("mirrors m1") || cmds.has("mirrors m3") || cmds.has("mirrors m4") || cmds.has("mega_tap1") {
		t.Errorf("removed rows that are not stale: %q", cmds.cmds)
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

	pid := fmt.Sprintf(`external_ids:minimega-pid="%d"`, os.Getpid())
	for _, want := range []string{`external_ids:minimega-namespace="exp"`, `external_ids:minimega-kind="tap-mirror"`, pid} {
		if !cmds.has(want) {
			t.Errorf("mirror without %v: %q", want, cmds.cmds)
		}
	}
}
