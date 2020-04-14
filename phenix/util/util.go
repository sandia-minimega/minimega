package util

import (
	"io"
	"phenix/types"

	"github.com/olekukonko/tablewriter"
)

func PrintTableOfConfigs(writer io.Writer, configs types.Configs) error {
	table := tablewriter.NewWriter(writer)

	table.SetHeader([]string{"Kind", "Version", "Name"})

	for _, c := range configs {
		table.Append([]string{c.Kind, c.Version, c.Metadata.Name})
	}

	table.Render()
	return nil
}
