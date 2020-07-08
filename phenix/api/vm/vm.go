package vm

import (
	"errors"
	"fmt"
	"regexp"

	"phenix/api/experiment"
	"phenix/internal/mm"
	"phenix/types"
)

var vlanAliasRegex = regexp.MustCompile(`(.*) \(\d*\)`)

// List collects VMs, combining topology settings with running VM details if the
// experiment is running. It returns a slice of VM structs and any errors
// encountered while gathering them.
func List(expName string) ([]types.VM, error) {
	exp, err := experiment.Get(expName)
	if err != nil {
		return nil, fmt.Errorf("getting experiment %s: %w", expName, err)
	}

	var (
		running = make(map[string]types.VM)
		vms     []types.VM
	)

	if exp.Status.Running() {
		for _, vm := range mm.GetVMInfo(mm.NS(expName)) {
			running[vm.Name] = vm
		}
	}

	for idx, node := range exp.Spec.Topology.Nodes {
		vm := types.VM{
			ID:         idx,
			Name:       node.General.Hostname,
			Experiment: exp.Spec.ExperimentName,
			CPUs:       node.Hardware.VCPU,
			RAM:        node.Hardware.Memory,
			Disk:       node.Hardware.Drives[0].Image,
			Interfaces: make(map[string]string),
		}

		for _, iface := range node.Network.Interfaces {
			vm.IPv4 = append(vm.IPv4, iface.Address)
			vm.Networks = append(vm.Networks, iface.VLAN)
			vm.Interfaces[iface.VLAN] = iface.Address
		}

		if details, ok := running[vm.Name]; ok {
			vm.Host = details.Host
			vm.Running = details.Running
			vm.Networks = details.Networks
			vm.Taps = details.Taps
			vm.Uptime = details.Uptime

			// Reset slice of IPv4 addresses so we can be sure to align them
			// correctly with minimega networks below.
			vm.IPv4 = make([]string, len(details.Networks))

			// Since we get the IP from the experiment config, but the network name
			// from minimega (to preserve iface to network ordering), make sure the
			// ordering of IPs matches the odering of networks. We could just use a
			// map here, but then the iface to network ordering that minimega ensures
			// would be lost.
			for idx, nw := range details.Networks {
				// At this point, `nw` will look something like `EXP_1 (101)`. In the
				// experiment config, we just have `EXP_1` so we need to use that
				// portion from minimega as the `Interfaces` map key.
				if match := vlanAliasRegex.FindStringSubmatch(nw); match != nil {
					vm.IPv4[idx] = vm.Interfaces[match[1]]
				} else {
					vm.IPv4[idx] = "n/a"
				}
			}
		}

		vms = append(vms, vm)
	}

	return vms, nil
}

// Get retrieves the VM with the given name from the experiment with the given
// name. If the experiment is running, topology VM settings are combined with
// running VM details. It returns a pointer to a VM struct, and any errors
// encountered while retrieving the VM.
func Get(expName, vmName string) (*types.VM, error) {
	exp, err := experiment.Get(expName)
	if err != nil {
		return nil, fmt.Errorf("getting experiment %s: %w", expName, err)
	}

	var vm *types.VM

	for idx, node := range exp.Spec.Topology.Nodes {
		if node.General.Hostname != vmName {
			continue
		}

		vm = &types.VM{
			ID:         idx,
			Name:       node.General.Hostname,
			Experiment: exp.Spec.ExperimentName,
			CPUs:       node.Hardware.VCPU,
			RAM:        node.Hardware.Memory,
			Disk:       node.Hardware.Drives[0].Image,
			Interfaces: make(map[string]string),
		}

		for _, iface := range node.Network.Interfaces {
			vm.IPv4 = append(vm.IPv4, iface.Address)
			vm.Networks = append(vm.Networks, iface.VLAN)
			vm.Interfaces[iface.VLAN] = iface.Address
		}
	}

	if vm == nil {
		return nil, fmt.Errorf("VM %s not found in experiment %s", vmName, expName)
	}

	if !exp.Status.Running() {
		return vm, nil
	}

	details := mm.GetVMInfo(mm.NS(expName), mm.VM(vmName))

	if len(details) != 1 {
		return vm, nil
	}

	vm.Host = details[0].Host
	vm.Running = details[0].Running
	vm.Networks = details[0].Networks
	vm.Taps = details[0].Taps
	vm.Uptime = details[0].Uptime

	// Reset slice of IPv4 addresses so we can be sure to align them
	// correctly with minimega networks below.
	vm.IPv4 = nil

	// Since we get the IP from the database, but the network name
	// from minimega (to preserve iface to network ordering), make
	// sure the ordering of IPs matches the odering of networks. We
	// could just use a map here, but then the iface to network
	// ordering that minimega ensures would be lost.
	for _, nw := range details[0].Networks {
		// At this point, `nw` will look something like `EXP_1 (101)`.
		// In the database, we just have `EXP_1` so we need to use
		// that portion from minimega as the `Interfaces` map key.
		if match := vlanAliasRegex.FindStringSubmatch(nw); match != nil {
			vm.IPv4 = append(vm.IPv4, vm.Interfaces[match[1]])
		}
	}

	return vm, nil
}

// Pause stops a running VM with the given name in the experiment with the given
// name. It returns any errors encountered while pausing the VM.
func Pause(expName, vmName string) error {
	err := StopCaptures(expName, vmName)
	if err != nil && !errors.Is(err, ErrNoCaptures) {
		return fmt.Errorf("stopping captures for VM %s in experiment %s: %w", vmName, expName, err)
	}

	if err := mm.StopVM(mm.NS(expName), mm.VM(vmName)); err != nil {
		return fmt.Errorf("pausing VM: %w", err)
	}

	return nil
}

// Resume starts a paused VM with the given name in the experiment with the
// given name. It returns any errors encountered while resuming the VM.
func Resume(expName, vmName string) error {
	if err := mm.StartVM(mm.NS(expName), mm.VM(vmName)); err != nil {
		return fmt.Errorf("resuming VM: %w", err)
	}

	return nil
}

// Redeploy redeploys a VM with the given name in the experiment with the given
// name. Multiple redeploy options can be passed to alter the resulting
// redeployed VM, such as CPU, memory, and disk options. It returns any errors
// encountered while redeploying the VM.
func Redeploy(expName, vmName string, opts ...RedeployOption) error {
	o := newRedeployOptions(opts...)

	var injects []string

	if o.inject {
		exp, err := experiment.Get(expName)
		if err != nil {
			return fmt.Errorf("getting experiment %s: %w", expName, err)
		}

		for _, n := range exp.Spec.Topology.Nodes {
			if n.General.Hostname != vmName {
				continue
			}

			if o.disk == "" {
				o.disk = n.Hardware.Drives[0].Image
				o.part = n.Hardware.Drives[0].GetInjectPartition()
			}

			for _, i := range n.Injections {
				injects = append(injects, fmt.Sprintf("%s:%s", i.Src, i.Dst))
			}

			break
		}
	}

	mmOpts := []mm.Option{
		mm.NS(expName),
		mm.VM(vmName),
		mm.CPU(o.cpu),
		mm.Mem(o.mem),
		mm.Disk(o.disk),
		mm.Injects(injects...),
		mm.InjectPartition(o.part),
	}

	if err := mm.RedeployVM(mmOpts...); err != nil {
		return fmt.Errorf("redeploying VM: %w", err)
	}

	return nil
}

// Kill deletes a VM with the given name in the experiment with the given name.
// It returns any errors encountered while killing the VM.
func Kill(expName, vmName string) error {
	if err := mm.KillVM(mm.NS(expName), mm.VM(vmName)); err != nil {
		return fmt.Errorf("killing VM: %w", err)
	}

	return nil
}
