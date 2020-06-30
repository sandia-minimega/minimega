package app

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"phenix/types"
	v1 "phenix/types/version/v1"
)

func TestSerialApp(t *testing.T) {
	baseDir, err := ioutil.TempDir("", "serial-app-test")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	defer os.RemoveAll(baseDir)

	// minimal spec for testing serial app
	nodes := []*v1.Node{
		{
			General: v1.General{
				Hostname: "linux-serial-node",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_Linux,
			},
			Network: v1.Network{
				Interfaces: []v1.Interface{
					{
						Type: "serial",
					},
				},
			},
		},
		{
			General: v1.General{
				Hostname: "linux-node",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_Linux,
			},
			Network: v1.Network{
				Interfaces: []v1.Interface{
					{
						Type: "ethernet",
					},
				},
			},
		},
		{
			General: v1.General{
				Hostname: "windows-serial-node",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_Windows,
			},
			Network: v1.Network{
				Interfaces: []v1.Interface{
					{
						Type: "serial",
					},
				},
			},
		},
	}

	// first slice of 2D slice represents topology node
	expected := [][]v1.Injection{
		{
			{
				Src: fmt.Sprintf("%s/startup/linux-serial-node-serial.bash", baseDir),
				Dst: "/etc/phenix/serial-startup.bash",
			},
			{
				Src: baseDir + "/startup/serial-startup.service",
				Dst: "/etc/systemd/system/serial-startup.service",
			},
			{
				Src: baseDir + "/startup/symlinks/serial-startup.service",
				Dst: "/etc/systemd/system/multi-user.target.wants/serial-startup.service",
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

	exp := &types.Experiment{Spec: spec}

	app := GetApp("serial")

	if err := app.Configure(exp); err != nil {
		t.Log(err)
		t.FailNow()
	}

	checkConfigureExpected(t, nodes, expected)

	if err := app.PreStart(exp); err != nil {
		t.Log(err)
		t.FailNow()
	}

	checkStartExpected(t, nodes, expected)
}
