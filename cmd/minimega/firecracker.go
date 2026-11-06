// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

const (
	MinVsockCID = 3
	MaxVsockCID = 1<<32 - 2
)

var errFirecrackerNotSupported = errors.New("firecracker VMs are not supported yet")

type FirecrackerConfig struct {
	// Set the Firecracker binary to use. The value is a path on the host or
	// the name of a binary in the host PATH.
	//
	// Note: this configuration only applies to Firecracker-based VMs.
	//
	// Default: "firecracker"
	FirecrackerPath string `config:"firecracker-path" path:"false"`

	// Set the Firecracker jailer binary to use. The value is a path on the
	// host or the name of a binary in the host PATH. The jailer must have the
	// same version as the Firecracker binary.
	//
	// Note: this configuration only applies to Firecracker-based VMs.
	//
	// Default: "jailer"
	JailerPath string `config:"jailer-path" path:"false"`

	// Enable the PCI transport for the devices of a VM. This sets the
	// --enable-pci flag of Firecracker. With the PCI transport, a VM can have
	// up to 31 devices. Without it, the limit is 17 devices. The guest kernel
	// must support PCI.
	//
	// Note: this configuration only applies to Firecracker-based VMs.
	//
	// Default: false
	FirecrackerPCI bool `config:"firecracker-pci"`

	// Add a virtio entropy device to a VM. The guest kernel can use this
	// device as a source of random numbers.
	//
	// Note: this configuration only applies to Firecracker-based VMs.
	//
	// Default: true
	FirecrackerEntropy bool `config:"firecracker-entropy"`

	// Set the guest context ID (CID) of the vsock device of a VM. Each VM has
	// its own vsock device, so all VMs can use the same CID. The value must be
	// in the range 3 to 4294967294.
	//
	// Note: this configuration only applies to Firecracker-based VMs.
	//
	// Default: 3
	VsockCID uint64 `config:"vsock-cid" validate:"validVsockCID"`

	// Set the path prefix of a Firecracker snapshot that vm save wrote. When
	// set, vm launch restores the VM from the snapshot instead of a cold
	// boot.
	//
	// Note: this configuration only applies to Firecracker-based VMs.
	FirecrackerSnapshotPath string `config:"firecracker-snapshot" path:"false"`

	// Files to inject into the first disk at launch, handler is not generated
	// by vmconfiger.
	Injects FCInjects `config:"inject"`
}

type FirecrackerVM struct {
	*BaseVM           // embed
	KVMConfig         // embed; kernel, initrd, append, and disks
	FirecrackerConfig // embed
}

// Ensure that FirecrackerVM implements the VM interface.
var _ VM = (*FirecrackerVM)(nil)

func (old FirecrackerConfig) Copy() FirecrackerConfig {
	res := old

	if old.Injects != nil {
		res.Injects = make(FCInjects, len(old.Injects))
		copy(res.Injects, old.Injects)
	}

	return res
}

func (vm *FirecrackerConfig) String() string {
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Firecracker configuration:")
	fmt.Fprintf(w, "Firecracker Path:\t%v\n", vm.FirecrackerPath)
	fmt.Fprintf(w, "Jailer Path:\t%v\n", vm.JailerPath)
	fmt.Fprintf(w, "PCI:\t%v\n", vm.FirecrackerPCI)
	fmt.Fprintf(w, "Entropy:\t%v\n", vm.FirecrackerEntropy)
	fmt.Fprintf(w, "Vsock CID:\t%v\n", vm.VsockCID)
	fmt.Fprintf(w, "Snapshot:\t%v\n", vm.FirecrackerSnapshotPath)
	fmt.Fprintf(w, "Injects:\t%v\n", vm.Injects)
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

func validVsockCID(_ VMConfig, cid uint64) error {
	if cid < MinVsockCID || cid > MaxVsockCID {
		return fmt.Errorf("vsock-cid must be in range %d-%d", MinVsockCID, MaxVsockCID)
	}

	return nil
}

func NewFirecracker(name, namespace string, config VMConfig) (*FirecrackerVM, error) {
	vm := new(FirecrackerVM)

	var err error
	vm.BaseVM, err = NewBaseVM(name, namespace, config)
	if err != nil {
		return nil, err
	}

	vm.Type = FIRECRACKER

	vm.KVMConfig = config.KVMConfig.Copy()
	vm.FirecrackerConfig = config.FirecrackerConfig.Copy()

	return vm, nil
}

func (vm *FirecrackerVM) Copy() VM {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm2 := new(FirecrackerVM)

	// Make shallow copies of all fields.
	*vm2 = *vm

	// Make deep copies.
	vm2.BaseVM = vm.BaseVM.copy()
	vm2.KVMConfig = vm.KVMConfig.Copy()
	vm2.FirecrackerConfig = vm.FirecrackerConfig.Copy()

	return vm2
}

func (vm *FirecrackerVM) Launch() error {
	defer vm.lock.Unlock()

	// Create the instance directory so that the VM can enter ERROR and be
	// flushed.
	if err := os.MkdirAll(vm.instancePath, os.FileMode(0700)); err != nil {
		vm.State = VM_ERROR
		return fmt.Errorf("unable to create VM dir: %v", err)
	}

	if err := vm.createInstancePathAlias(); err != nil {
		return vm.setErrorf("createInstancePathAlias: %v", err)
	}

	return vm.setErrorf("%v", errFirecrackerNotSupported)
}

func (vm *FirecrackerVM) Recover(string, int) error {
	defer vm.lock.Unlock()

	return errors.New("recovery is not supported for firecracker VMs")
}

func (vm *FirecrackerVM) Start() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State&VM_RUNNING != 0 {
		return nil
	}

	return errFirecrackerNotSupported
}

func (vm *FirecrackerVM) Stop() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State != VM_RUNNING {
		return vmNotRunning(vm.Name)
	}

	return errFirecrackerNotSupported
}

func (vm *FirecrackerVM) Flush() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	return vm.BaseVM.Flush()
}

func (vm *FirecrackerVM) String() string {
	return fmt.Sprintf("%s:%d:firecracker", hostname, vm.ID)
}

func (vm *FirecrackerVM) Info(field string) (string, error) {
	// Let BaseVM answer generic fields first.
	if v, err := vm.BaseVM.Info(field); err == nil {
		return v, nil
	}

	vm.lock.Lock()
	defer vm.lock.Unlock()

	if v, err := vm.FirecrackerConfig.Info(field); err == nil {
		return v, nil
	}

	return vm.KVMConfig.Info(field)
}

func (vm *FirecrackerVM) Conflicts(vm2 VM) error {
	switch vm2 := vm2.(type) {
	case *FirecrackerVM:
		return vm.BaseVM.conflicts(vm2.BaseVM)
	case *KvmVM:
		return vm.BaseVM.conflicts(vm2.BaseVM)
	case *ContainerVM:
		return vm.BaseVM.conflicts(vm2.BaseVM)
	case *AndroidVM:
		return vm.BaseVM.conflicts(vm2.BaseVM)
	}

	return errors.New("unknown VM type")
}

func (vm *FirecrackerVM) Screenshot(size int) ([]byte, error) {
	return nil, errors.New("screenshots are not supported for firecracker VMs")
}

func (vm *FirecrackerVM) Connect(cc *ron.Server, reconnect bool) error {
	return nil
}

func (vm *FirecrackerVM) Disconnect(cc *ron.Server) error {
	return nil
}

func (vm *FirecrackerVM) ProcStats() (map[int]*ProcStats, error) {
	if vm.Pid <= 0 {
		return nil, errors.New("firecracker VM has no PID")
	}

	p, err := GetProcStats(vm.Pid)
	if err != nil {
		return nil, err
	}

	return map[int]*ProcStats{vm.Pid: p}, nil
}

func (vm *FirecrackerVM) WriteConfig(w io.Writer) error {
	if err := vm.BaseConfig.WriteConfig(w); err != nil {
		return err
	}

	if err := vm.KVMConfig.WriteConfig(w); err != nil {
		return err
	}

	return vm.FirecrackerConfig.WriteConfig(w)
}
