package util

import (
	"phenix/types"
	"phenix/web/proto"
	wtypes "phenix/web/types"
)

func ExperimentToProtobuf(exp types.Experiment, status wtypes.Status, vms []types.VM) *proto.Experiment {
	pb := &proto.Experiment{
		Name:      exp.Spec.ExperimentName,
		Topology:  exp.Metadata.Annotations["topology"],
		StartTime: exp.Status.StartTime,
		Running:   exp.Status.Running(),
		Status:    string(status),
		VlanMin:   uint32(exp.Spec.VLANs.Min),
		VlanMax:   uint32(exp.Spec.VLANs.Max),
		Vms:       VMsToProtobuf(vms),
	}

	var apps []string

	for _, app := range exp.Spec.Scenario.Apps.Experiment {
		apps = append(apps, app.Name)
	}

	for _, app := range exp.Spec.Scenario.Apps.Host {
		apps = append(apps, app.Name)
	}

	pb.Apps = apps

	var vlans []*proto.VLAN

	for alias := range exp.Spec.VLANs.Aliases {
		vlan := &proto.VLAN{
			Vlan:  uint32(exp.Spec.VLANs.Aliases[alias]),
			Alias: alias,
		}

		vlans = append(vlans, vlan)
	}

	pb.Vlans = vlans

	return pb
}

func VMToProtobuf(vm types.VM) *proto.VM {
	return &proto.VM{
		Name:        vm.Name,
		Host:        vm.Host,
		Ipv4:        vm.IPv4,
		Cpus:        uint32(vm.CPUs),
		Ram:         uint32(vm.RAM),
		Disk:        vm.Disk,
		Uptime:      vm.Uptime,
		Networks:    vm.Networks,
		Taps:        vm.Taps,
		Captures:    CapturesToProtobuf(vm.Captures),
		DoNotBoot:   vm.DoNotBoot,
		Screenshot:  vm.Screenshot,
		Running:     vm.Running,
		Redeploying: vm.Redeploying,
	}
}

func VMsToProtobuf(vms []types.VM) []*proto.VM {
	pb := make([]*proto.VM, len(vms))

	for i, vm := range vms {
		pb[i] = VMToProtobuf(vm)
	}

	return pb
}

func CaptureToProtobuf(capture types.Capture) *proto.Capture {
	return &proto.Capture{
		Vm:        capture.VM,
		Interface: uint32(capture.Interface),
		Filepath:  capture.Filepath,
	}
}

func CapturesToProtobuf(captures []types.Capture) []*proto.Capture {
	pb := make([]*proto.Capture, len(captures))

	for i, capture := range captures {
		pb[i] = CaptureToProtobuf(capture)
	}

	return pb
}

func ExperimentScheduleToProtobuf(exp types.Experiment) *proto.ExperimentSchedule {
	var sched []*proto.Schedule

	for vm, host := range exp.Spec.Schedules {
		sched = append(sched, &proto.Schedule{Vm: vm, Host: host})
	}

	return &proto.ExperimentSchedule{Schedule: sched}
}
