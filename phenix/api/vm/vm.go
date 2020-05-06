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
			vm.IPv4 = nil

			// Since we get the IP from the database, but the network name
			// from minimega (to preserve iface to network ordering), make
			// sure the ordering of IPs matches the odering of networks. We
			// could just use a map here, but then the iface to network
			// ordering that minimega ensures would be lost.
			for _, nw := range details.Networks {
				// At this point, `nw` will look something like `EXP_1 (101)`.
				// In the database, we just have `EXP_1` so we need to use
				// that portion from minimega as the `Interfaces` map key.
				if match := vlanAliasRegex.FindStringSubmatch(nw); match != nil {
					vm.IPv4 = append(vm.IPv4, vm.Interfaces[match[1]])
				}
			}
		}

		vms = append(vms, vm)
	}

	return vms, nil
}

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

func Pause(expName, vmName string) error {
	err := StopVMCaptures(expName, vmName)
	if err != nil && !errors.Is(err, ErrNoCaptures) {
		return fmt.Errorf("stopping captures for VM %s in experiment %s: %w", vmName, expName, err)
	}

	if err := mm.StopVM(mm.NS(expName), mm.VM(vmName)); err != nil {
		return fmt.Errorf("pausing VM: %w", err)
	}

	return nil
}

func Resume(expName, vmName string) error {
	if err := mm.StartVM(mm.NS(expName), mm.VM(vmName)); err != nil {
		return fmt.Errorf("resuming VM: %w", err)
	}

	return nil
}

func Kill(expName, vmName string) error {
	return nil
}

func Redeploy(expName, vmName string, inject bool) error {
	return nil
}
