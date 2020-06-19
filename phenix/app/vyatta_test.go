package app

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	v1 "phenix/types/version/v1"
)

func TestVyattaApp(t *testing.T) {
	baseDir, err := ioutil.TempDir("", "vyatta-app-test")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	defer os.RemoveAll(baseDir)

	nodes := []*v1.Node{
		{
			Type: "Router",
			General: v1.General{
				Hostname: "router",
			},
		},
		{
			Type: "VirtualMachine",
			Labels: v1.Labels{
				"ntp-server": "true",
			},
			General: v1.General{
				Hostname: "linux",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_Linux,
			},
		},
		{
			Type: "VirtualMachine",
			General: v1.General{
				Hostname: "win",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_Windows,
			},
		},
	}

	expected := [][]v1.Injection{
		{
			{
				Src: fmt.Sprintf("%s/vyatta/router.boot", baseDir),
				Dst: "/opt/vyatta/etc/config/config.boot",
			},
		},
		nil,
		nil,
	}

	spec := &v1.ExperimentSpec{
		BaseDir: baseDir,
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
	}

	app := GetApp("vyatta")

	if err := app.Configure(spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	checkConfigureExpected(t, nodes, expected)

	if err := app.Start(spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	checkStartExpected(t, nodes, expected)
}
