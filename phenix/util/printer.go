package util

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"phenix/internal/mm"
	"phenix/types"
	v1 "phenix/types/version/v1"

	"github.com/olekukonko/tablewriter"
)

// PrintTableOfConfigs writes the given configs to the given writer as an ASCII
// table. The table headers are set to Kind, Version, Name, and Created.
func PrintTableOfConfigs(writer io.Writer, configs types.Configs) {
	table := tablewriter.NewWriter(writer)

	table.SetHeader([]string{"Kind", "Version", "Name", "Created"})

	for _, c := range configs {
		table.Append([]string{c.Kind, c.Version, c.Metadata.Name, c.Metadata.Created})
	}

	table.Render()
}

// PrintTableOfExperiments writes the given experiments to the given writer as
// an ASCII table. The table headers are set to Name, Topology, Scenario,
// Started, VM Count, VLAN Count, and Apps.
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
			fmt.Sprintf("%d", len(exp.Spec.Topology.Nodes)),
			fmt.Sprintf("%d", len(exp.Spec.VLANs.Aliases)),
			strings.Join(apps, ", "),
		})

	}

	table.Render()
}

// PrintTableOfVMs writes the given VMs to the given writer as an ASCII table.
// The table headers are set to Host, Name, Running, Disk, Interfaces, and
// Uptime.
func PrintTableOfVMs(writer io.Writer, vms ...mm.VM) {
	table := tablewriter.NewWriter(writer)

	table.SetHeader([]string{"Host", "Name", "Running", "Disk", "Interfaces", "Uptime"})
	table.SetAutoWrapText(false)
	table.SetColWidth(50)

	for _, vm := range vms {
		var (
			running = strconv.FormatBool(vm.Running)
			ifaces  []string
			uptime  string
		)

		for idx, nw := range vm.Networks {
			ifaces = append(ifaces, fmt.Sprintf("ID: %d, IP: %s, VLAN: %s", idx, vm.IPv4[idx], nw))
		}

		if vm.Running {
			uptime = (time.Duration(vm.Uptime) * time.Second).String()
		}

		table.Append([]string{vm.Host, vm.Name, running, vm.Disk, strings.Join(ifaces, "\n"), uptime})
	}

	table.Render()
}

func PrintTableOfImageConfigs(writer io.Writer, optional []string, imgs ...types.Image) {
	var (
		table = tablewriter.NewWriter(writer)
		cols  = []string{"Name", "Size", "Variant", "Release", "Overlays", "Packages", "Scripts"}
	)

	cols = append(cols, optional...)

	table.SetHeader(cols)

	for _, img := range imgs {
		var scripts []string

		for s := range img.Spec.Scripts {
			scripts = append(scripts, s)
		}

		row := []string{
			img.Metadata.Name,
			img.Spec.Size,
			img.Spec.Variant,
			img.Spec.Release,
			strings.Join(img.Spec.Overlays, "\n"),
			strings.Join(img.Spec.Packages, "\n"),
			strings.Join(scripts, "\n"),
		}

		for _, col := range optional {
			switch col {
			case "Format":
				row = append(row, string(img.Spec.Format))
			case "Compressed":
				row = append(row, strconv.FormatBool(img.Spec.Compress))
			case "Mirror":
				row = append(row, img.Spec.Mirror)
			}
		}

		table.Append(row)
	}

	table.Render()
}

func PrintTableOfVLANAliases(writer io.Writer, info map[string]v1.VLANAliases) {
	table := tablewriter.NewWriter(writer)
	table.SetHeader([]string{"Experiment", "VLAN Alias", "VLAN ID"})

	var experiments []string

	for exp := range info {
		experiments = append(experiments, exp)
	}

	sort.Strings(experiments)

	for _, exp := range experiments {
		var aliases []string

		for alias := range info[exp] {
			aliases = append(aliases, alias)
		}

		sort.Strings(aliases)

		for _, alias := range aliases {
			table.Append([]string{exp, alias, fmt.Sprintf("%d", int(info[exp][alias]))})
		}
	}

	table.Render()
}

func PrintTableOfVLANRanges(writer io.Writer, info map[string][2]int) {
	table := tablewriter.NewWriter(writer)
	table.SetHeader([]string{"Experiment", "VLAN Range"})

	var experiments []string

	for exp := range info {
		experiments = append(experiments, exp)
	}

	sort.Strings(experiments)

	for _, exp := range experiments {
		r := fmt.Sprintf("%d - %d", info[exp][0], info[exp][1])

		table.Append([]string{exp, r})
	}

	table.Render()
}
