package util

import (
	"io"
	"phenix/types"
	"time"

	"github.com/olekukonko/tablewriter"
)

func PrintTableOfConfigs(writer io.Writer, configs types.Configs) error {
	table := tablewriter.NewWriter(writer)

	table.SetHeader([]string{"Kind", "Version", "Name", "Created"})

	for _, c := range configs {
		table.Append([]string{c.Kind, c.Version, c.Metadata.Name, c.Metadata.Created.Format(time.RFC3339)})
	}

	table.Render()
	return nil
}
