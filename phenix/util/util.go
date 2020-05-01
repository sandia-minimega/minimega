package util

import (
	"io"
	"os/exec"
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

func ShellCommandExists(cmd string) bool {
	err := exec.Command("which", cmd).Run()
	return err == nil
}
