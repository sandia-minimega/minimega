package vm

import (
	"fmt"
	"phenix/internal/mm"
)

// Connect moves or reconnects the given interface for the given VM in the given
// experiment to the given VLAN. The given interface must already exist in the
// VM. It returns any errors encountered while connecting the interface.
func Connect(expName, vmName string, iface int, vlan string) error {
	if expName == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if vmName == "" {
		return fmt.Errorf("no VM name provided")
	}

	err := mm.ConnectVMInterface(mm.NS(expName), mm.VM(vmName), mm.ConnectInterface(iface), mm.ConnectVLAN(vlan))
	if err != nil {
		return fmt.Errorf("connecting VM interface to VLAN: %w", err)
	}

	return nil
}

// Disconnect disconnects the given interface for the given VM in the given
// experiment from the VLAN it's currently connected to (if any). It returns any
// errors encountered while disconnecting the interface.
func Disonnect(expName, vmName string, iface int) error {
	if expName == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if vmName == "" {
		return fmt.Errorf("no VM name provided")
	}

	err := mm.DisconnectVMInterface(mm.NS(expName), mm.VM(vmName), mm.ConnectInterface(iface))
	if err != nil {
		return fmt.Errorf("disconnecting VM interface: %w", err)
	}

	return nil
}
