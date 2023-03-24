// Copyright 2016-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/sandia-minimega/minimega/v2/internal/recovery"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func recover() error {
	log.Info("attempting to recover any vms from previous minimega instances")

	if err := recoverVLANs(); err != nil {
		return fmt.Errorf("unable to recover VLANs: %w", err)
	}

	if err := recoverHostTaps(); err != nil {
		return fmt.Errorf("unable to recover host taps: %w", err)
	}

	namespaces, err := recovery.RecoverVMs(*f_base)
	if err != nil {
		return fmt.Errorf("unable to recover previous vms: %w", err)
	}

	for namespace, vms := range namespaces {
		log.Info("recovering vms for namespace %s", namespace)

		ns := GetOrCreateNamespace(namespace)

		for _, vm := range vms {
			body, err := ioutil.ReadFile(filepath.Join(*f_base, vm.VMID, "name"))
			if err != nil {
				return fmt.Errorf("unable to read name file for vm %s: %w", vm.VMID, err)
			}

			name := strings.TrimSpace(string(body))

			log.Info("recovering vm %s (ID: %s)", name, vm.VMID)

			f, err := os.Open(filepath.Join(*f_base, vm.VMID, "config"))
			if err != nil {
				return fmt.Errorf("unable to open config file for vm %s: %w", vm.VMID, err)
			}

			cfg := NewVMConfig()
			if err := cfg.ReadConfig(f, namespace); err != nil {
				f.Close()
				return fmt.Errorf("unable to read config for vm %s: %w", vm.VMID, err)
			}

			f.Close()

			if len(cfg.Networks) > 0 {
				body, err = ioutil.ReadFile(filepath.Join(*f_base, vm.VMID, "taps"))
				if err != nil {
					return fmt.Errorf("unable to read taps file for vm %s: %w", vm.VMID, err)
				}

				taps := strings.Split(string(body), "\n")

				// length of taps might be greater than length of network configs if
				// there's an empty line at the end of the file
				if len(taps) < len(cfg.Networks) {
					return fmt.Errorf("mismatch between tap and interface count for vm %s", vm.VMID)
				}

				for i, c := range cfg.Networks {
					c.Tap = taps[i]
					cfg.Networks[i] = c

					br, err := bridges.Get(c.Bridge)
					if err != nil {
						return fmt.Errorf("unable to get bridge %s: %w", c.Bridge, err)
					}

					br.RecoverTap(c.Tap, c.MAC, c.VLAN, false)
				}
			}

			if len(cfg.Bonds) > 0 {
				body, err = ioutil.ReadFile(filepath.Join(*f_base, vm.VMID, "bonds"))
				if err != nil {
					return fmt.Errorf("unable to read bonds file for vm %s: %w", vm.VMID, err)
				}

				bonds := strings.Split(string(body), "\n")

				// length of bond names might be greater than length of network
				// bonds if there's an empty line at the end of the file
				if len(bonds) < len(cfg.Bonds) {
					return fmt.Errorf("mismatch between bond name and bond count for vm %s", vm.VMID)
				}

				for i, b := range cfg.Bonds {
					b.Name = bonds[i]
					cfg.Bonds[i] = b

					br, err := bridges.Get(b.Bridge)
					if err != nil {
						return fmt.Errorf("unable to get bridge %s: %w", b.Bridge, err)
					}

					bonded := make(map[string]int)

					for _, idx := range b.Interfaces {
						n := cfg.Networks[idx]
						bonded[n.Tap] = n.VLAN
					}

					br.RecoverBond(b.Name, bonded)
				}
			}

			kvm, err := NewKVM(name, namespace, cfg)
			if err != nil {
				return fmt.Errorf("unable to create new vm %s: %w", vm.VMID, err)
			}

			// unlock the VM and reset VM ID and PID
			if err := kvm.Recover(vm.VMID, vm.PID); err != nil {
				return fmt.Errorf("unable to recover vm %s: %w", vm.VMID, err)
			}

			// add VM to namespace
			ns.VMs.m[kvm.ID] = kvm

			state, err := VMStateFromString(vm.State)
			if err != nil {
				return fmt.Errorf("unable to parse state for vm %s: %w", vm.VMID, err)
			}

			kvm.State = state

			if state == VM_RUNNING || state == VM_PAUSED {
				log.Info("finding process for vm %s (ID: %s)", name, vm.VMID)

				proc, err := os.FindProcess(vm.PID)
				if err != nil {
					return fmt.Errorf("unable to find QEMU process for vm %s: %w", vm.VMID, err)
				}

				// Channel to signal when the process has exited
				var wait = make(chan bool)

				kvm.waitForExit(proc, wait)

				log.Info("connecting to QEMU QMP for vm %s (ID: %s)", name, vm.VMID)

				if err := kvm.connectQMP(); err != nil {
					return fmt.Errorf("unable to connect QMP for vm %s: %w", vm.VMID, err)
				}

				go kvm.qmpLogger()

				log.Info("connecting to VNC for vm %s (ID: %s)", name, vm.VMID)

				if err := kvm.connectVNC(); err != nil {
					return fmt.Errorf("unable to connect VNC for vm %s: %w", vm.VMID, err)
				}

				kvm.waitToKill(proc, wait)
				kvm.Connect(ns.ccServer, false)
			}
		}
	}

	return nil
}
