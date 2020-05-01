package app

import (
	"fmt"
	"os"
	"strings"

	"phenix/tmpl"
	v1 "phenix/types/version/v1"
)

type Serial struct{}

func (Serial) Init(...Option) error {
	return nil
}

func (Serial) Name() string {
	return "serial"
}

func (Serial) Configure(spec *v1.ExperimentSpec) error {
	// loop through nodes
	for _, node := range spec.Topology.Nodes {
		// We only care about configuring serial interfaces on Linux VMs.
		// TODO: handle rhel and centos OS types.
		if node.Hardware.OSType != v1.OSType_Linux {
			continue
		}

		var serial bool

		// Loop through interface type to see if any of the interfaces are serial.
		for _, iface := range node.Network.Interfaces {
			if iface.Type == "serial" {
				serial = true
				break
			}
		}

		if serial {
			// update injections to include serial type (src and dst)
			serialFile := spec.BaseDir + "/startup/" + node.General.Hostname + "-serial.bash"

			a := &v1.Injection{
				Src:         serialFile,
				Dst:         "/etc/phenix/serial-startup.bash",
				Description: "",
			}

			b := &v1.Injection{
				Src:         spec.BaseDir + "/startup/serial-startup.service",
				Dst:         "/etc/systemd/system/serial-startup.service",
				Description: "",
			}

			c := &v1.Injection{
				Src:         spec.BaseDir + "/startup/symlinks/serial-startup.service",
				Dst:         "/etc/systemd/system/multi-user.target.wants/serial-startup.service",
				Description: "",
			}

			node.Injections = append(node.Injections, a, b, c)
		}
	}

	return nil
}

func (Serial) Start(spec *v1.ExperimentSpec) error {
	// loop through nodes
	for _, node := range spec.Topology.Nodes {
		// We only care about configuring serial interfaces on Linux VMs.
		// TODO: handle rhel and centos OS types.
		if node.Hardware.OSType != v1.OSType_Linux {
			continue
		}

		var serial []v1.Interface

		// Loop through interface type to see if any of the interfaces are serial.
		for _, iface := range node.Network.Interfaces {
			if iface.Type == "serial" {
				serial = append(serial, iface)
			}
		}

		if serial != nil {
			startupDir := spec.BaseDir + "/startup"

			if err := os.MkdirAll(startupDir, 0755); err != nil {
				return fmt.Errorf("creating experiment startup directory path: %w", err)
			}

			serialFile := startupDir + "/" + node.General.Hostname + "-serial.bash"

			if err := tmpl.CreateFileFromTemplate("serial_startup.tmpl", serial, serialFile); err != nil {
				return fmt.Errorf("generating serial script: %w", err)
			}

			if err := tmpl.RestoreAsset(startupDir, "serial-startup.service"); err != nil {
				return fmt.Errorf("restoring serial-startup.service: %w", err)
			}

			symlinksDir := startupDir + "/symlinks"

			if err := os.MkdirAll(symlinksDir, 0755); err != nil {
				return fmt.Errorf("creating experiment startup symlinks directory path: %w", err)
			}

			if err := os.Symlink("../serial-startup.service", symlinksDir+"/serial-startup.service"); err != nil {
				// Ignore the error if it was for the symlinked file already existing.
				if !strings.Contains(err.Error(), "file exists") {
					return fmt.Errorf("creating symlink for serial-startup.service: %w", err)
				}
			}
		}
	}

	return nil
}

func (Serial) PostStart(spec *v1.ExperimentSpec) error {
	return nil
}

func (Serial) Cleanup(spec *v1.ExperimentSpec) error {
	return nil
}
