// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"path/filepath"
)

// fcIgnoredLaunchSettings returns one warning for each container setting, for
// the Android settings, and for each optimization that a launch of Firecracker
// VMs with cfg ignores. hugepages is the hugepages path of the namespace.
func fcIgnoredLaunchSettings(cfg VMConfig, hugepages string) []string {
	var res []string

	if hugepages != "" {
		res = append(res, fmt.Sprintf("firecracker VMs ignore optimize hugepages %v: huge pages for Firecracker VMs are not supported", hugepages))
	}

	c := cfg.ContainerConfig
	for _, v := range []struct {
		set bool
		key string
	}{
		{c.FilesystemPath != "", "filesystem"},
		{c.Hostname != "", "hostname"},
		{len(c.Init) > 0 && !(len(c.Init) == 1 && c.Init[0] == "/init"), "init"},
		{c.Preinit != "", "preinit"},
		{c.Fifos != 0, "fifos"},
		{len(c.VolumePaths) > 0, "volume"},
	} {
		if v.set {
			res = append(res, fmt.Sprintf("firecracker VMs ignore vm config %v", v.key))
		}
	}

	if androidConfigured(cfg.AndroidConfig) {
		res = append(res, "firecracker VMs ignore the vm config android settings")
	}

	return res
}

// fcSerialTail returns the last limit bytes of the console log of vm.
func fcSerialTail(vm *FirecrackerVM, limit int) ([]byte, error) {
	path, err := vm.Info("console")
	if err != nil {
		return nil, err
	}

	out, err := fcTail(path, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("unable to read serial port 0 for VM %s: %v", vm.GetName(), err)
	}

	return out, nil
}

// fcRecoveredID returns the VM ID and true when the instance directory of a
// VM that recovery found is a Firecracker instance directory. vmid is the path
// of the instance directory relative to base.
func fcRecoveredID(base, vmid string) (string, bool) {
	dir := fcResolve(filepath.Join(base, vmid))
	if filepath.Dir(dir) != filepath.Join(fcResolve(*f_firecrackerDir), "vm") {
		return "", false
	}

	return filepath.Base(dir), true
}
