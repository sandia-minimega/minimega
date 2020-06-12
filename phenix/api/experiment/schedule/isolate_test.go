package schedule

import (
	"testing"

	"phenix/internal/mm"
	"phenix/types"
	v1 "phenix/types/version/v1"

	"github.com/golang/mock/gomock"
)

func TestIsolateSchedulerManual(t *testing.T) {
	sched := v1.Schedule{
		"foo": "compute0",
	}

	spec := &v1.ExperimentSpec{
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
		Schedules: sched,
	}

	hosts := types.Hosts(
		[]types.Host{
			{
				Name:     "compute0",
				CPUs:     16,
				MemTotal: 49152,
			},
			{
				Name:     "compute1",
				CPUs:     16,
				MemTotal: 49152,
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mm.NewMockMM(ctrl)
	m.EXPECT().GetClusterHosts().Return(hosts, nil)

	mm.DefaultMM = m

	if err := Schedule("isolate-experiment", spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	for vm, host := range spec.Schedules {
		if host != "compute0" {
			t.Logf("expected %s -> compute0, got %s -> %s", vm, vm, host)
			t.FailNow()
		}
	}
}

func TestIsolateSchedulerFits(t *testing.T) {
	spec := &v1.ExperimentSpec{
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
		Schedules: v1.Schedule{},
	}

	hosts := types.Hosts(
		[]types.Host{
			{
				Name:     "compute1",
				CPUs:     1,
				MemTotal: 1024,
			},
			{
				Name:     "compute2",
				CPUs:     16,
				MemTotal: 49152,
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mm.NewMockMM(ctrl)
	m.EXPECT().GetClusterHosts().Return(hosts, nil)

	mm.DefaultMM = m

	if err := Schedule("isolate-experiment", spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	for vm, host := range spec.Schedules {
		if host != "compute2" {
			t.Logf("expected %s -> compute2, got %s -> %s", vm, vm, host)
			t.FailNow()
		}
	}
}

func TestIsolateSchedulerUnoccupied(t *testing.T) {
	spec := &v1.ExperimentSpec{
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
		Schedules: v1.Schedule{},
	}

	hosts := types.Hosts(
		[]types.Host{
			{
				Name:     "compute1",
				CPUs:     16,
				MemTotal: 49152,
				VMs:      1,
			},
			{
				Name:     "compute2",
				CPUs:     16,
				MemTotal: 49152,
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mm.NewMockMM(ctrl)
	m.EXPECT().GetClusterHosts().Return(hosts, nil)

	mm.DefaultMM = m

	if err := Schedule("isolate-experiment", spec); err != nil {
		t.Log(err)
		t.FailNow()
	}

	for vm, host := range spec.Schedules {
		if host != "compute2" {
			t.Logf("expected %s -> compute2, got %s -> %s", vm, vm, host)
			t.FailNow()
		}
	}
}

func TestIsolateSchedulerAllOccupied(t *testing.T) {
	spec := &v1.ExperimentSpec{
		Topology: &v1.TopologySpec{
			Nodes: nodes,
		},
		Schedules: v1.Schedule{},
	}

	hosts := types.Hosts(
		[]types.Host{
			{
				Name:     "compute1",
				CPUs:     16,
				MemTotal: 49152,
				VMs:      1,
			},
			{
				Name:     "compute2",
				CPUs:     16,
				MemTotal: 49152,
				VMs:      1,
			},
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mm.NewMockMM(ctrl)
	m.EXPECT().GetClusterHosts().Return(hosts, nil)

	mm.DefaultMM = m

	if err := Schedule("isolate-experiment", spec); err == nil {
		t.Log("expected error")
		t.FailNow()
	}
}
