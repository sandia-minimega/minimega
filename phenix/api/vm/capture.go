package vm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"phenix/api/experiment"
	"phenix/internal/mm"
)

var (
	ErrCaptureExists = errors.New("capture already exists")
	ErrNoCaptures    = errors.New("no captures exist")
)

func StartVMCapture(expName, vmName string, iface int, out string) error {
	vm, err := Get(expName, vmName)
	if err != nil {
		return fmt.Errorf("getting VM details: %w", err)
	}

	if !vm.Running {
		return fmt.Errorf("VM is not running")
	}

	if iface < 0 || iface >= len(vm.Networks) {
		return fmt.Errorf("invalid interface provided for capture")
	}

	if vm.Networks[iface] == "disconnected" {
		return fmt.Errorf("cannot capture on a disconnected interface")
	}

	captures := mm.GetVMCaptures(mm.NS(expName), mm.VM(vmName))

	for _, capture := range captures {
		if capture.Interface == iface {
			return fmt.Errorf("starting VM capture for interface %d on VM %s in experiment %s: %w", iface, vmName, expName, err)
		}
	}

	if ext := filepath.Ext(out); ext != ".pcap" {
		out = out + ".pcap"
	}

	if err := mm.StartVMCapture(mm.NS(expName), mm.VM(vmName), mm.CaptureInterface(iface), mm.CaptureFile(out)); err != nil {
		return fmt.Errorf("starting VM capture for interface %d on VM %s in experiment %s: %w", iface, vmName, expName, err)
	}

	return nil
}

func StopVMCaptures(expName, vmName string) error {
	captures := mm.GetVMCaptures(mm.NS(expName), mm.VM(vmName))

	if captures == nil {
		return fmt.Errorf("VM %s in experiment %s: %w", vmName, expName, ErrNoCaptures)
	}

	exp, err := experiment.Get(expName)
	if err != nil {
		return fmt.Errorf("getting experiment %s: %w", expName, err)
	}

	dir := fmt.Sprintf("%s/images/%s/files", exp.Spec.BaseDir, expName)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating files directory for experiment %s: %w", expName, err)
	}

	if err := mm.StopVMCapture(mm.NS(expName), mm.VM(vmName)); err != nil {
		return fmt.Errorf("stopping VM captures for VM %s in experiment %s: %w", vmName, expName, err)
	}

	for _, capture := range captures {
		base := filepath.Base(capture.Filepath)

		if err := os.Rename(capture.Filepath, dir+"/"+base); err != nil {
			return fmt.Errorf("moving capture file %s for interface %d on VM %s in experiment %s: %w", base, capture.Interface, vmName, expName, err)
		}
	}

	return nil
}
