package app

import (
	"fmt"
	"os"
	"path/filepath"

	"phenix/internal/common"
	"phenix/tmpl"
	"phenix/types"
	ifaces "phenix/types/interfaces"
)

type Startup struct{}

func (Startup) Init(...Option) error {
	return nil
}

func (Startup) Name() string {
	return "startup"
}

func (this *Startup) Configure(exp *types.Experiment) error {
	startupDir := exp.Spec.BaseDir() + "/startup"

	for _, node := range exp.Spec.Topology().Nodes() {
		// if type is router, skip it and continue
		if node.Type() == "Router" {
			continue
		}

		var keep []ifaces.NodeInjection

		// delete any exisitng interface injections
		for _, inject := range node.Injections() {
			if inject.Dst() == "interfaces" || inject.Dst() == "startup.ps1" {
				continue
			}

			keep = append(keep, inject)
		}

		node.SetInjections(keep)

		// loop through nodes
		if node.Hardware().OSType() == "linux" || node.Hardware().OSType() == "rhel" || node.Hardware().OSType() == "centos" {
			// if vm is centos or rhel, need a separate file per interface
			if node.Hardware().OSType() == "rhel" || node.Hardware().OSType() == "centos" {
				for idx := range node.Network().Interfaces() {
					node.AddInject(
						fmt.Sprintf("%s/interfaces-%s-eth%d", startupDir, node.General().Hostname(), idx),
						fmt.Sprintf("/etc/sysconfig/network-scripts/ifcfg-eth%d", idx),
						"", "",
					)
				}
			} else if node.Hardware().OSType() == "linux" {
				var (
					hostnameFile = startupDir + "/" + node.General().Hostname() + "-hostname.sh"
					timezoneFile = startupDir + "/" + node.General().Hostname() + "-timezone.sh"
					ifaceFile    = startupDir + "/" + node.General().Hostname() + "-interfaces"
				)

				node.AddInject(
					hostnameFile,
					"/etc/phenix/startup/1_hostname-start.sh",
					"0755", "",
				)

				node.AddInject(
					timezoneFile,
					"/etc/phenix/startup/2_timezone-start.sh",
					"0755", "",
				)

				node.AddInject(
					ifaceFile,
					"/etc/network/interfaces",
					"", "",
				)
			}
		} else if node.Hardware().OSType() == "windows" {
			var (
				startupFile = startupDir + "/" + node.General().Hostname() + "-startup.ps1"
				schedFile   = startupDir + "/startup-scheduler.cmd"
			)

			node.AddInject(
				startupFile,
				"startup.ps1",
				"0755", "",
			)

			node.AddInject(
				schedFile,
				"ProgramData/Microsoft/Windows/Start Menu/Programs/StartUp/startup_scheduler.cmd",
				"0755", "",
			)
		}
	}

	return nil
}

func (this Startup) PreStart(exp *types.Experiment) error {
	// note in the mako file that there does not appear to be timezone or hostname for rhel and centos
	startupDir := exp.Spec.BaseDir() + "/startup"
	imageDir := common.PhenixBase + "/images/"

	if err := os.MkdirAll(startupDir, 0755); err != nil {
		return fmt.Errorf("creating experiment startup directory path: %w", err)
	}

	for _, node := range exp.Spec.Topology().Nodes() {
		// Check if user provided an absolute path to image. If not, prepend path
		// with default image path.
		imagePath := node.Hardware().Drives()[0].Image()

		if !filepath.IsAbs(imagePath) {
			imagePath = imageDir + imagePath
		}

		// check if the disk image is present, if not set do not boot to true
		if _, err := os.Stat(imagePath); os.IsNotExist(err) {
			node.General().SetDoNotBoot(true)
		}

		// if type is router, skip it and continue
		if node.Type() == "Router" {
			continue
		}

		// it appears linux, rhel, and centos share the same interfaces template
		if node.Hardware().OSType() == "rhel" || node.Hardware().OSType() == "centos" {
			for idx := range node.Network().Interfaces() {
				ifaceFile := fmt.Sprintf("%s/interfaces-%s-eth%d", startupDir, node.General().Hostname(), idx)

				if err := tmpl.CreateFileFromTemplate("linux_interfaces.tmpl", node, ifaceFile); err != nil {
					return fmt.Errorf("generating linux interfaces config: %w", err)
				}
			}
		} else if node.Hardware().OSType() == "linux" {
			var (
				hostnameFile = startupDir + "/" + node.General().Hostname() + "-hostname.sh"
				timezoneFile = startupDir + "/" + node.General().Hostname() + "-timezone.sh"
				ifaceFile    = startupDir + "/" + node.General().Hostname() + "-interfaces"
				timeZone     = "Etc/UTC"
			)

			if err := tmpl.CreateFileFromTemplate("linux_hostname.tmpl", node.General().Hostname(), hostnameFile); err != nil {
				return fmt.Errorf("generating linux hostname config: %w", err)
			}

			if err := tmpl.CreateFileFromTemplate("linux_timezone.tmpl", timeZone, timezoneFile); err != nil {
				return fmt.Errorf("generating linux timezone config: %w", err)
			}

			if err := tmpl.CreateFileFromTemplate("linux_interfaces.tmpl", node, ifaceFile); err != nil {
				return fmt.Errorf("generating linux interfaces config: %w", err)
			}
		} else if node.Hardware().OSType() == "windows" {
			// Temporary struct to send to the Windows Startup template.
			data := struct {
				Node     ifaces.NodeSpec
				Metadata map[string]interface{}
			}{
				Node:     node,
				Metadata: make(map[string]interface{}),
			}

			// Check to see if a scenario exists for this experiment and if it
			// contains a "startup" app. If so, see if this node has a metadata entry
			// in the scenario app configuration.
			for _, app := range exp.Apps() {
				if app.Name() == "startup" {
					for _, host := range app.Hosts() {
						if host.Hostname() == node.General().Hostname() {
							data.Metadata = host.Metadata()
						}
					}
				}
			}

			startupFile := startupDir + "/" + node.General().Hostname() + "-startup.ps1"

			if err := tmpl.CreateFileFromTemplate("windows_startup.tmpl", data, startupFile); err != nil {
				return fmt.Errorf("generating windows startup config: %w", err)
			}

			if err := tmpl.RestoreAsset(startupDir, "startup-scheduler.cmd"); err != nil {
				return fmt.Errorf("restoring windows startup scheduler: %w", err)
			}
		}
	}

	return nil
}

func (Startup) PostStart(exp *types.Experiment) error {
	return nil
}

func (Startup) Cleanup(exp *types.Experiment) error {
	return nil
}
