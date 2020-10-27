package app

import (
	"fmt"
	"os"
	"strings"

	"phenix/tmpl"
	"phenix/types"
)

type NTP struct{}

func (NTP) Init(...Option) error {
	return nil
}

func (NTP) Name() string {
	return "ntp"
}

func (this *NTP) Configure(exp *types.Experiment) error {
	ntpServers := exp.Spec.Topology().FindNodesWithLabels("ntp-server")

	if len(ntpServers) != 0 {
		// Just take first server if more than one are labeled.
		node := ntpServers[0]

		ntpDir := exp.Spec.BaseDir() + "/ntp"
		ntpFile := ntpDir + "/" + node.General().Hostname() + "_ntp"

		if err := os.MkdirAll(ntpDir, 0755); err != nil {
			return fmt.Errorf("creating experiment ntp directory path: %w", err)
		}

		if node.Type() == "Router" {
			node.AddInject(ntpFile, "/opt/vyatta/etc/ntp.conf", "", "")
		} else if node.Hardware().OSType() == "linux" {
			node.AddInject(ntpFile, "/etc/ntp.conf", "", "")
		} else if node.Hardware().OSType() == "windows" {
			node.AddInject(ntpFile, "ntp.ps1", "0755", "")
		}
	}

	return nil
}

func (this NTP) PreStart(exp *types.Experiment) error {
	ntpServers := exp.Spec.Topology().FindNodesWithLabels("ntp-server")

	if len(ntpServers) != 0 {
		// Just take first server if more than one are labeled.
		node := ntpServers[0]

		var ntpAddr string

		for _, iface := range node.Network().Interfaces() {
			if strings.EqualFold(iface.VLAN(), "mgmt") {
				ntpAddr = iface.Address()
				break
			}
		}

		ntpDir := exp.Spec.BaseDir() + "/ntp"
		ntpFile := ntpDir + "/" + node.General().Hostname() + "_ntp"

		if node.Type() == "Router" {
			if err := tmpl.CreateFileFromTemplate("ntp_linux.tmpl", ntpAddr, ntpFile); err != nil {
				return fmt.Errorf("generating ntp script: %w", err)
			}
		} else if node.Hardware().OSType() == "linux" {
			if err := tmpl.CreateFileFromTemplate("ntp_linux.tmpl", ntpAddr, ntpFile); err != nil {
				return fmt.Errorf("generating ntp script: %w", err)
			}
		} else if node.Hardware().OSType() == "windows" {
			if err := tmpl.CreateFileFromTemplate("ntp_windows.tmpl", ntpAddr, ntpFile); err != nil {
				return fmt.Errorf("generating ntp script: %w", err)
			}
		}
	}

	return nil
}

func (NTP) PostStart(exp *types.Experiment) error {
	return nil
}

func (NTP) Cleanup(exp *types.Experiment) error {
	return nil
}
