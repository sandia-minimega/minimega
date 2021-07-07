// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
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
				if !strings.Contains(column, "vnc_host") {
					res["host"] = resp.Host
					continue
				}
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

	for resps := range run(cmd) {
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
