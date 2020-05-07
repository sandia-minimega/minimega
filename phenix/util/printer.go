package util

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"phenix/types"

	"github.com/olekukonko/tablewriter"
)

func PrintTableOfConfigs(writer io.Writer, configs types.Configs) {
	table := tablewriter.NewWriter(writer)

	table.SetHeader([]string{"Kind", "Version", "Name", "Created"})

	for _, c := range configs {
		table.Append([]string{c.Kind, c.Version, c.Metadata.Name, c.Metadata.Created})
	}

	table.Render()
}

func PrintTableOfExperiments(writer io.Writer, exps ...types.Experiment) {
	table := tablewriter.NewWriter(writer)

	table.SetHeader([]string{"Name", "Topology", "Scenario", "Started", "VM Count", "VLAN Count", "Apps"})

	for _, exp := range exps {
		var apps []string

		if exp.Spec.Scenario != nil && exp.Spec.Scenario.Apps != nil {
			for _, app := range exp.Spec.Scenario.Apps.Experiment {
				apps = append(apps, app.Name)
			}

			for _, app := range exp.Spec.Scenario.Apps.Host {
				apps = append(apps, app.Name)
			}
		}

		table.Append([]string{
			exp.Spec.ExperimentName,
			exp.Metadata.Annotations["topology"],
			exp.Metadata.Annotations["scenario"],
			exp.Status.StartTime,
			strconv.Itoa(len(exp.Spec.Topology.Nodes)),
			"",
			strings.Join(apps, ", "),
		})

	}

	table.Render()
}

func PrintTableOfVMs(writer io.Writer, vms ...types.VM) {
	table := tablewriter.NewWriter(writer)

	table.SetHeader([]string{"Host", "Name", "Running", "Disk", "Interfaces", "Uptime"})

	for _, vm := range vms {
		var (
			running = strconv.FormatBool(vm.Running)
			ifaces  []string
			uptime  string
		)

		for idx, nw := range vm.Networks {
			ifaces = append(ifaces, fmt.Sprintf("%s - %s", vm.IPv4[idx], nw))
		}

		if vm.Running {
			uptime = (time.Duration(vm.Uptime) * time.Second).String()
		}

		table.Append([]string{vm.Host, vm.Name, running, vm.Disk, strings.Join(ifaces, "\n"), uptime})
	}

	table.Render()
}
