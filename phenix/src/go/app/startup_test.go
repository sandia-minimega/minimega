package app

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"phenix/types"
	v1 "phenix/types/version/v1"
)

func TestStartupApp(t *testing.T) {
	baseDir, err := ioutil.TempDir("", "startup-app-test")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	defer os.RemoveAll(baseDir)

	nodes := []*v1.Node{
		{
			Type: "Router",
		},
		{
			Type: "VirtualMachine",
			General: v1.General{
				Hostname: "centos-linux",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_CentOS,
			},
			Network: v1.Network{
				Interfaces: []v1.Interface{
					{}, // empty interface for testing
					{}, // empty interface for testing
				},
			},
		},
		{
			Type: "VirtualMachine",
			General: v1.General{
				Hostname: "rhel-linux",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_RHEL,
			},
			Network: v1.Network{
				Interfaces: []v1.Interface{
					{}, // empty interface for testing
					{}, // empty interface for testing
					{}, // empty interface for testing
				},
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
			Injections: []*v1.Injection{
				{
					Dst: "interfaces",
				},
			},
		},
		{
			Type: "VirtualMachine",
			General: v1.General{
				Hostname: "windows",
			},
			Hardware: v1.Hardware{
				OSType: v1.OSType_Windows,
			},
			Injections: []*v1.Injection{
				{
					Dst: "startup.ps1",
				},
			},
		},
	}

	expected := [][]v1.Injection{
		nil, // router
		{ // centos-linux
			{
				Src: fmt.Sprintf("%s/startup/interfaces-centos-linux-eth0", baseDir),
				Dst: "/etc/sysconfig/network-scripts/ifcfg-eth0",
			},
			{
				Src: fmt.Sprintf("%s/startup/interfaces-centos-linux-eth1", baseDir),
				Dst: "/etc/sysconfig/network-scripts/ifcfg-eth1",
			},
		},
		{ // rhel-linux
			{
				Src: fmt.Sprintf("%s/startup/interfaces-rhel-linux-eth0", baseDir),
				Dst: "/etc/sysconfig/network-scripts/ifcfg-eth0",
			},
			{
				Src: fmt.Sprintf("%s/startup/interfaces-rhel-linux-eth1", baseDir),
				Dst: "/etc/sysconfig/network-scripts/ifcfg-eth1",
			},
			{
				Src: fmt.Sprintf("%s/startup/interfaces-rhel-linux-eth2", baseDir),
				Dst: "/etc/sysconfig/network-scripts/ifcfg-eth2",
			},
		},
		{ // linux
			{
				Src: fmt.Sprintf("%s/startup/linux-hostname.sh", baseDir),
				Dst: "/etc/phenix/startup/1_hostname-start.sh",
			},
			{
				Src: fmt.Sprintf("%s/startup/linux-timezone.sh", baseDir),
				Dst: "/etc/phenix/startup/2_timezone-start.sh",
			},
			{
				Src: fmt.Sprintf("%s/startup/linux-interfaces", baseDir),
				Dst: "/etc/network/interfaces",
			},
		},
		{ // windows
			{
				Src: fmt.Sprintf("%s/startup/windows-startup.ps1", baseDir),
				Dst: "startup.ps1",
			},
			{
				Src: fmt.Sprintf("%s/startup/startup-scheduler.cmd", baseDir),
				Dst: "ProgramData/Microsoft/Windows/Start Menu/Programs/StartUp/startup_scheduler.cmd",
			},
		},
	}

	spec := &v1.ExperimentSpec{
		BaseDir: baseDir,
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
	}

	exp := &types.Experiment{Spec: spec}

	app := GetApp("startup")

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
