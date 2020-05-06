package util

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"phenix/types"

	"github.com/olekukonko/tablewriter"
)

func PrintTableOfConfigs(writer io.Writer, configs types.Configs) error {
	table := tablewriter.NewWriter(writer)

	table.SetHeader([]string{"Kind", "Version", "Name", "Created"})

	for _, c := range configs {
		table.Append([]string{c.Kind, c.Version, c.Metadata.Name, c.Metadata.Created})
	}

	table.Render()

	return nil
}

func PrintTableOfVMs(writer io.Writer, vms ...types.VM) error {
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

	return nil
}

func ShellCommandExists(cmd string) bool {
	err := exec.Command("which", cmd).Run()
	return err == nil
}
