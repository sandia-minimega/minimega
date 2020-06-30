package app

import (
	"fmt"
	"os"
	"strings"

	"phenix/tmpl"
	"phenix/types"
	v1 "phenix/types/version/v1"
)

type Vyatta struct{}

func (Vyatta) Init(...Option) error {
	return nil
}

func (Vyatta) Name() string {
	return "vyatta"
}

func (Vyatta) Configure(exp *types.Experiment) error {
	// loop through nodes
	for _, node := range exp.Spec.Topology.Nodes {
		if !strings.EqualFold(node.Type, "router") {
			continue
		}

		vyattaFile := exp.Spec.BaseDir + "/vyatta/" + node.General.Hostname + ".boot"

		a := &v1.Injection{
			Src:         vyattaFile,
			Dst:         "/opt/vyatta/etc/config/config.boot",
			Description: "",
		}

		node.Injections = append(node.Injections, a)
	}

	return nil
}

func (Vyatta) PreStart(exp *types.Experiment) error {
	var (
		ntpServers = exp.Spec.Topology.FindNodesWithLabels("ntp-server")
		ntpAddr    string
	)

	if len(ntpServers) != 0 {
		// Just take first server if more than one are labeled.
		for _, iface := range ntpServers[0].Network.Interfaces {
			if strings.EqualFold(iface.VLAN, "mgmt") {
				ntpAddr = iface.Address
				break
			}
		}
	}

	// loop through nodes
	for _, node := range exp.Spec.Topology.Nodes {
		if !strings.EqualFold(node.Type, "router") {
			continue
		}

		data := map[string]interface{}{
			"node":     node,
			"ntp-addr": ntpAddr,
		}

		vyattaDir := exp.Spec.BaseDir + "/vyatta"

		if err := os.MkdirAll(vyattaDir, 0755); err != nil {
			return fmt.Errorf("creating experiment vyatta directory path: %w", err)
		}

		vyattaFile := vyattaDir + "/" + node.General.Hostname + ".boot"

		if err := tmpl.CreateFileFromTemplate("vyatta.tmpl", data, vyattaFile); err != nil {
			return fmt.Errorf("generating vyatta config: %w", err)
		}
	}

	return nil
}

func (Vyatta) PostStart(exp *types.Experiment) error {
	return nil
}

func (Vyatta) Cleanup(exp *types.Experiment) error {
	return nil
}
