package app

import (
	"fmt"
	"os"
	"phenix/tmpl"
	v1 "phenix/types/version/v1"
)

type Startup struct{}

func (Startup) Init(...Option) error {
	return nil
}

func (Startup) Name() string {
	return "startup"
}

func (this *Startup) Configure(spec *v1.ExperimentSpec) error {
	startupDir := spec.BaseDir + "/startup"

	for _, node := range spec.Topology.Nodes {
		// loop through nodes
		if node.Hardware.OSType == v1.OSType_Linux || node.Hardware.OSType == v1.OSType_RHEL || node.Hardware.OSType == v1.OSType_CentOS {
			// if type is router, continue
			if node.Type == "Router" {
				continue
			}

			// delete any exisitng interface injections
			for i, inject := range node.Injections {
				if inject.Destination == "interfaces" || inject.Destination == "startup.ps1" {
					copy(inject[i:], inject[i+1:])
					inject[len(inject)-1] = ""
					inject = inject[:len(inject)-1]
				}
			}

			// if vm is centos or rhel, need a separate file per interface
			if node.Hardware.OSType == v1.OSType_RHEL || node.Hardware.OSType == v1.OSType_CentOS {
				for idx, iface := range node.Network.Interfaces {
					ifaceFile := startupDir + "/interfaces-" + node.General.Hostname + "-eth" + idx

					a := &v1.Injection{
						Src:         ifaceFile,
						Dst:         "/etc/sysconfig/nettwork-scripts/ifcfg-eth" + idx,
						Description: "",
					}

					node.Injections = append(node.Injections, a)
				}
			} else if node.Hardware.OSType == v1.OSType_Linux {
				hostnameFile := startupDir + "/" + node.General.Hostname + "-hostname.sh"
				timezoneFile := startupDir + "/" + node.General.Hostname + "-timezone.sh"
				ifaceFile := startupDir + "/" + node.General.Hostname + "-interfaces.sh"

				a := &v1.Injection{
					Src:         hostnameFile,
					Dst:         "/etc/phenix/startup/1_hostname-start.sh",
					Description: "",
				}
				b := &v1.Injection{
					Src:         timezoneFile,
					Dst:         "/etc/phenix/startup/2_timezone-start.sh",
					Description: "",
				}
				c := &v1.Injection{
					Src:         ifaceFile,
					Dst:         "/etc/network/interfaces",
					Description: "",
				}

				node.Injections = append(node.Injections, a, b, c)
			} else if node.Hardware.OSType == v1.OSType_Windows {
				startupFile := startupDir + "/" + node.General.Hostname + "-startup.ps1"
				schedFile := startupDir + "/" + node.General.Hostname + "-startup-scheduler.cmd"

				a := &v1.Injection{
					Src:         startupFile,
					Dst:         "startup.ps1",
					Destination: "",
				}
				b := &v1.Injection{
					Src:         schedFile,
					Dst:         "ProgramData/Microsoft/Windows/Start Menu/Programs/StartUp/startup_scheduler.cmd",
					Description: "",
				}

				node.Injections = append(node.Injections, a, b)
			}
		}
	}

	return nil
}

func (this Startup) Start(spec *v1.ExperimentSpec) error {
	// note in the mako file that there does not appear to be timezone or hostname for rhel and centos
	startupDir := spec.BaseDir + "/startup"

	if err := os.MkdirAll(startupDir, 0755); err != nil {
		return fmt.Errorf("creating experiment startup directory path: %w", err)
	}

	for _, node := range spec.Topology.Nodes {
		// it appears linux, rhel, and centos share the same interfaces template
		if node.Hardware.OSType == v1.OSType_Linux || node.Hardware.OSType == v1.OSType_RHEL || node.Hardware.OSType == v1.OSType_CentOS {
			ifaceFile := startupDir + "/interfaces-" + node.General.Hostname + "-eth" + idx

			if err := tmpl.CreateFileFromTemplate("linux_interfaces.tmpl", node, ifaceFile); err != nil {
				return fmt.Errorf("generating linux interfaces config: %w", err)
			}
		} else if node.Hardware.OSType == v1.OSType_Linux {
			hostnameFile := startupDir + "/" + node.General.Hostname + "-hostname.sh"
			timezoneFile := startupDir + "/" + node.General.Hostname + "-timezone.sh"

			if err := tmpl.CreateFileFromTemplate("linux_hostname.tmpl", node.General.Hostname, hostnameFile); err != nil {
				return fmt.Errorf("generating linux hostname config: %w", err)
			}

			timeZone := "Etc/UTC"

			if err := tmpl.CreateFileFromTemplate("linux_timezone.tmpl", timeZone, timezoneFile); err != nil {
				return fmt.Errorf("generating linux interfaces config: %w", err)
			}
		} else if node.Hardware.OSType == v1.OSType_Windows {
			startupFile := startupDir + "/" + node.General.Hostname + "-startup.ps1"
			schedFile := startupDir + "/" + node.General.Hostname + "-startup-scheduler.cmd"

			if err := tmpl.CreateFileFromTemplate("windows_startup.tmpl", node, startupFile); err != nil {
				return fmt.Errorf("generating linux hostname config: %w", err)
			}

			if err := tmpl.CreateFileFromTemplate("windows_startup_scheduler.tmpl", node, schedFile); err != nil {
				return fmt.Errorf("generating linux hostname config: %w", err)
			}
		}
	}

	return nil
}

func (Startup) PostStart(spec *v1.ExperimentSpec) error {
	return nil
}

func (Startup) Cleanup(spec *v1.ExperimentSpec) error {
	return nil
}
