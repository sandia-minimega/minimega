package scheduler

import (
	"testing"

	"phenix/internal/mm"
	v1 "phenix/types/version/v1"

	"github.com/golang/mock/gomock"
)

func TestSubnetComputeSchedulerNoCommits(t *testing.T) {
	spec := &v1.ExperimentSpec{
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
		Schedules: v1.Schedule{},
	}

	hosts := mm.Hosts(
		[]mm.Host{
			{
				Name:      "compute0",
				MemCommit: 0,
				MemTotal:  49152,
			},
			{
				Name:      "compute1",
				MemCommit: 0,
				MemTotal:  49152,
			},
			{
				Name:      "compute2",
				MemCommit: 0,
				MemTotal:  49152,
			},
			{
				Name:      "compute3",
				MemCommit: 0,
				MemTotal:  49152,
			},
			{
				Name:      "compute4",
				MemCommit: 0,
				MemTotal:  49152,
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mm.NewMockMM(ctrl)
	m.EXPECT().GetClusterHosts().Return(hosts, nil)

	mm.DefaultMM = m

	if err := Schedule("subnet-compute", spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	expected := v1.Schedule{
		"foo":   "compute0",
		"bar":   "compute1",
		"sucka": "compute0",
		"fish":  "compute1",
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

func TestSubnetComputeSchedulerSomePrescheduled(t *testing.T) {
	spec := &v1.ExperimentSpec{
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
		Schedules: v1.Schedule{
			"bar": "compute4",
		},
	}

	hosts := mm.Hosts(
		[]mm.Host{
			{
				Name:      "compute0",
				MemCommit: 0,
				MemTotal:  49152,
			},
			{
				Name:      "compute1",
				MemCommit: 0,
				MemTotal:  49152,
			},
			{
				Name:      "compute2",
				MemCommit: 0,
				MemTotal:  49152,
			},
			{
				Name:      "compute3",
				MemCommit: 0,
				MemTotal:  49152,
			},
			{
				Name:      "compute4",
				MemCommit: 0,
				MemTotal:  49152,
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mm.NewMockMM(ctrl)
	m.EXPECT().GetClusterHosts().Return(hosts, nil)

	mm.DefaultMM = m

	if err := Schedule("subnet-compute", spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	expected := v1.Schedule{
		"foo":   "compute0",
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
