package app

import (
	"fmt"
	"os"

	"phenix/tmpl"
	v1 "phenix/types/version/v1"
)

type NTP struct{}

func (NTP) Init(...Option) error {
	return nil
}

func (NTP) Name() string {
	return nil
}

func (NTP) Configure(spec *v1.ExperimentSpec) error {
	return nil
}

func (NTP) Start(spec *v1.ExperimentSpec) error {

	for _, node := range spec.Topology.Nodes {
		if node.General.Hostname == "*elk*" {
			if node.Interface.VLAN == "MGMT" { // do we need to worry about case
				elk_addr := node.Interface.Address
			} else {
				elk_addr := "172.16.0.254"
			}
		}
		ntp_addr := elk_addr

		if node.General.Hostname == "*ntp*" { 
			if node.Interface.VLAN == "MGMT" { 
				ntp_addr = node.Interface.Address
			} 
		}

		ntpDir := spec.BaseDir + "/ntp"
		ntpFile := ntpDir + "/" + node.General.Hostname + "_ntp"

		if err := os.MkdirAll(ntpDir, 0755); err != nil {
			return fmt.Errorf("creating experiment ntp directory path: %w", err)
		}

		if node.Type == "Router" {
			a := v1.Injection{
				Src:	ntpFile,
				Dst:	"/opt/vyatta/etc/ntp.conf",
				Description: "",
			}
			
			node.Injections = append(node.Injections, a)
			
			if err := tmpl.CreateFileFromTemplate("ntp_linux.tmpl", ntp_addr, ntpFile); err != nil {
				return fmt.Errorf("generating ntp script: %w", err)
		} else if node.Hardware.OSType == "linux" {
			a := v1.Injection{
				Src:	ntpFile,
				Dst:	"/etc/ntp.conf",
				Description: "",
			}
			
			node.Injections = append(node.Injections, a)
			
			if err := tmpl.CreateFileFromTemplate("ntp_linux.tmpl", ntp_addr, ntpFile); err != nil {
				return fmt.Errorf("generating ntp script: %w", err)
			}
		} else {
			a := v1.Injection{
				Src:	ntpFile,
				Dst:	"ntp.ps1",
				Description: "",
			}
			
			node.Injections = append(node.Injections, a)
			
			if err := tmpl.CreateFileFromTemplate("ntp_windows.tmpl", ntp_addr, ntpFile); err != nil {
				return fmt.Errorf("generating ntp script: %w", err)
		}

	}

	return nil
}

func (NTP) PostStart(spec *v1.ExperimentSpec) error {
	return nil
}

func (NTP) Cleanup(spec *v1.ExperimentSpec) error {
	return nil
}
