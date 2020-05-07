package vm

import (
	"fmt"
	"phenix/internal/mm"
)

func Connect(expName, vmName string, iface int, vlan string) error {
	err := mm.ConnectVMInterface(mm.NS(expName), mm.VM(vmName), mm.ConnectInterface(iface), mm.ConnectVLAN(vlan))
	if err != nil {
		return fmt.Errorf("connecting VM interface to VLAN: %w", err)
	}

	return nil
}

func Disonnect(expName, vmName string, iface int) error {
	err := mm.DisonnectVMInterface(mm.NS(expName), mm.VM(vmName), mm.ConnectInterface(iface))
	if err != nil {
		return fmt.Errorf("disconnecting VM interface: %w", err)
	}

	return nil
}
