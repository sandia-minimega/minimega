package scheduler

import (
	"testing"

	"phenix/internal/mm"
	"phenix/types"
	v1 "phenix/types/version/v1"

	"github.com/golang/mock/gomock"
)

func TestRoundRobinSchedulerNoVMs(t *testing.T) {
	spec := &v1.ExperimentSpec{
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
		Schedules: v1.Schedule{},
	}

	hosts := types.Hosts(
		[]types.Host{
			{
				Name: "compute0",
				VMs:  0,
			},
			{
				Name: "compute1",
				VMs:  0,
			},
			{
				Name: "compute2",
				VMs:  0,
			},
			{
				Name: "compute3",
				VMs:  0,
			},
			{
				Name: "compute4",
				VMs:  0,
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mm.NewMockMM(ctrl)
	m.EXPECT().GetClusterHosts().Return(hosts, nil)

	mm.DefaultMM = m

	if err := Schedule("round-robin", spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	expected := v1.Schedule{
		"foo":   "compute0",
		"bar":   "compute1",
		"sucka": "compute2",
		"fish":  "compute3",
	}

	if len(spec.Schedules) != len(expected) {
		t.Logf("expected %d VMs to be scheduled, got %d", len(expected), len(spec.Schedules))
		t.FailNow()
	}

	for vm, host := range expected {
		if spec.Schedules[vm] != host {
			t.Logf("expected %s -> %s, got %s -> %s", vm, host, vm, spec.Schedules[vm])
			t.FailNow()
		}
	}
}

func TestRoundRobinSchedulerSomeVMs(t *testing.T) {
	spec := &v1.ExperimentSpec{
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
		Schedules: v1.Schedule{},
	}

	hosts := types.Hosts(
		[]types.Host{
			{
				Name: "compute0",
				VMs:  0,
			},
			{
				Name: "compute1",
				VMs:  3,
			},
			{
				Name: "compute2",
				VMs:  2,
			},
			{
				Name: "compute3",
				VMs:  0,
			},
			{
				Name: "compute4",
				VMs:  0,
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mm.NewMockMM(ctrl)
	m.EXPECT().GetClusterHosts().Return(hosts, nil)

	mm.DefaultMM = m

	if err := Schedule("round-robin", spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	expected := v1.Schedule{
		"foo":   "compute0",
		"bar":   "compute3",
		"sucka": "compute4",
		"fish":  "compute4",
	}

	if len(spec.Schedules) != len(expected) {
		t.Logf("expected %d VMs to be scheduled, got %d", len(expected), len(spec.Schedules))
		t.FailNow()
	}

	for vm, host := range expected {
		if spec.Schedules[vm] != host {
			t.Logf("expected %s -> %s, got %s -> %s", vm, host, vm, spec.Schedules[vm])
			t.FailNow()
		}
	}
}

func TestRoundRobinSchedulerSomePrescheduled(t *testing.T) {
	spec := &v1.ExperimentSpec{
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
		Schedules: v1.Schedule{
			"sucka": "compute0",
		},
	}

	hosts := types.Hosts(
		[]types.Host{
			{
				Name: "compute0",
				VMs:  0,
			},
			{
				Name: "compute1",
				VMs:  3,
			},
			{
				Name: "compute2",
				VMs:  2,
			},
			{
				Name: "compute3",
				VMs:  0,
			},
			{
				Name: "compute4",
				VMs:  0,
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mm.NewMockMM(ctrl)
	m.EXPECT().GetClusterHosts().Return(hosts, nil)

	mm.DefaultMM = m

	if err := Schedule("round-robin", spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	expected := v1.Schedule{
		"foo":   "compute3",
		"bar":   "compute4",
		"sucka": "compute0",
		"fish":  "compute4",
	}

	if len(spec.Schedules) != len(expected) {
		t.Logf("expected %d VMs to be scheduled, got %d", len(expected), len(spec.Schedules))
		t.FailNow()
	}

	for vm, host := range expected {
		if spec.Schedules[vm] != host {
			t.Logf("expected %s -> %s, got %s -> %s", vm, host, vm, spec.Schedules[vm])
			t.FailNow()
		}
	}
}
