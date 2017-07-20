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

func runTabular(cmd string, columns, filters []string) []map[string]string {
	// apply filters first so we don't need to worry about the columns not
	// including the filtered fields.
	for _, f := range filters {
		cmd = fmt.Sprintf(".filter %v %v", f, cmd)
	}

	// copy all fields in header order
	mapper := tabularToMap

	if len(columns) > 0 {
		// replace mapper to only copy fields in column order
		mapper = tabularToMapCols(columns)

		// quote all the columns in case there are spaces
		for i, c := range columns {
			columns[i] = fmt.Sprintf("%q", c)
		}

		cmd = fmt.Sprintf(".columns %v %v", strings.Join(columns, ","), cmd)
	}

	// don't record command in history
	cmd = fmt.Sprintf(".record false %v", cmd)

	res := []map[string]string{}

	for resps := range mm.Run(cmd) {
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
