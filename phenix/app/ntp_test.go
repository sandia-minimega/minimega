package app

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"phenix/types"
	v1 "phenix/types/version/v1"
)

func TestNTPAppRouter(t *testing.T) {
	baseDir, err := ioutil.TempDir("", "ntp-app-test")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	defer os.RemoveAll(baseDir)

	nodes := []*v1.Node{
		{
			Type: "Router",
			Labels: v1.Labels{
				"ntp-server": "true",
			},
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
			Labels: v1.Labels{
				"ntp-server": "true",
			},
			General: v1.General{
				Hostname: "win",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_Windows,
			},
		},
	}

	// only first node w/ ntp-server tag should be configured
	expected := [][]v1.Injection{
		{
			{
				Src: fmt.Sprintf("%s/ntp/router_ntp", baseDir),
				Dst: "/opt/vyatta/etc/ntp.conf",
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

	app := GetApp("ntp")

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

func TestNTPAppLinux(t *testing.T) {
	baseDir, err := ioutil.TempDir("", "ntp-app-test")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	defer os.RemoveAll(baseDir)

	nodes := []*v1.Node{
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
			Labels: v1.Labels{
				"ntp-server": "true",
			},
			General: v1.General{
				Hostname: "win",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_Windows,
			},
		},
		{
			Type: "Router",
			Labels: v1.Labels{
				"ntp-server": "true",
			},
			General: v1.General{
				Hostname: "router",
			},
		},
	}

	// only first node w/ ntp-server tag should be configured
	expected := [][]v1.Injection{
		{
			{
				Src: fmt.Sprintf("%s/ntp/linux_ntp", baseDir),
				Dst: "/etc/ntp.conf",
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

	app := GetApp("ntp")

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

func TestNTPAppWindows(t *testing.T) {
	baseDir, err := ioutil.TempDir("", "ntp-app-test")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	defer os.RemoveAll(baseDir)

	nodes := []*v1.Node{
		{
			Type: "VirtualMachine",
			Labels: v1.Labels{
				"ntp-server": "true",
			},
			General: v1.General{
				Hostname: "win",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_Windows,
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
			Type: "Router",
			Labels: v1.Labels{
				"ntp-server": "true",
			},
			General: v1.General{
				Hostname: "router",
			},
		},
	}

	// only first node w/ ntp-server tag should be configured
	expected := [][]v1.Injection{
		{
			{
				Src: fmt.Sprintf("%s/ntp/win_ntp", baseDir),
				Dst: "ntp.ps1",
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

	app := GetApp("ntp")

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

func TestNTPAppNone(t *testing.T) {
	baseDir, err := ioutil.TempDir("", "ntp-app-test")
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

	// no ntp-server labels present
	expected := [][]v1.Injection{nil, nil, nil}

	spec := &v1.ExperimentSpec{
		BaseDir: baseDir,
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
	}

	exp := &types.Experiment{Spec: spec}

	app := GetApp("ntp")

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
