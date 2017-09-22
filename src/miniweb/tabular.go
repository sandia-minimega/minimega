// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"strings"
)

type Command struct {
	Command   string
	Columns   []string
	Filters   []string
	Namespace string
}

func (c *Command) String() string {
	cmd := c.Command

	// apply filters first so we don't need to worry about the columns not
	// including the filtered fields.
	for _, f := range c.Filters {
		cmd = fmt.Sprintf(".filter %v %v", f, cmd)
	}

	if len(c.Columns) > 0 {
		columns := make([]string, len(c.Columns))

		// quote all the columns in case there are spaces
		for i := range c.Columns {
			columns[i] = fmt.Sprintf("%q", c.Columns[i])
		}

		cmd = fmt.Sprintf(".columns %v %v", strings.Join(columns, ","), cmd)
	}

	// if there's a namespace, use it
	if c.Namespace != "" {
		cmd = fmt.Sprintf("namespace %q %v", c.Namespace, cmd)
	}

	// don't record command in history
	cmd = fmt.Sprintf(".record false %v", cmd)

	log.Debug("built command: `%v`", cmd)
	return cmd
}

type tabularToMapper func(*minicli.Response, []string) map[string]string

func tabularToMap(resp *minicli.Response, row []string) map[string]string {
	res := map[string]string{
		"host": resp.Host,
	}

	for i, header := range resp.Header {
		res[header] = row[i]
	}

	return res
}

func tabularToMapCols(columns []string) tabularToMapper {
	// create local copy of columns in case they get changed
	cols := make([]string, len(columns))
	copy(cols, columns)

	return func(resp *minicli.Response, row []string) map[string]string {
		res := map[string]string{}
		for _, column := range cols {
			if strings.Contains(column, "host") {
				res["host"] = resp.Host
				continue
			}

			for i, header := range resp.Header {
				if strings.Contains(column, header) {
					res[header] = row[i]
				}
			}
		}
		return res
	}
}

func runTabular(cmd *Command) []map[string]string {
	// copy all fields in header order
	mapper := tabularToMap

	if len(cmd.Columns) > 0 {
		// replace mapper to only copy fields in column order
		mapper = tabularToMapCols(cmd.Columns)
	}

	res := []map[string]string{}

	for resps := range mm.Run(cmd.String()) {
		for _, resp := range resps.Resp {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			for _, row := range resp.Tabular {
				res = append(res, mapper(resp, row))
			}
		}
	}

	return res
}
