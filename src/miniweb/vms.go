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

func vmInfo(columns, filters []string) []map[string]string {
	cmd := "vm info"

	// apply filters first so we don't need to worry about the columns not
	// including the filtered fields.
	for _, f := range filters {
		cmd = fmt.Sprintf(".filter %v %v", f, cmd)
	}

	// quote all the columns in case there are spaces
	for i, c := range columns {
		columns[i] = fmt.Sprintf("%q", c)
	}

	// copy all fields in header order
	doVM := func(resp *minicli.Response, row []string) map[string]string {
		vm := map[string]string{
			"host": resp.Host,
		}

		for i, header := range resp.Header {
			vm[header] = row[i]
		}

		return vm
	}

	if len(columns) > 0 {
		cmd = fmt.Sprintf(".columns %v %v", strings.Join(columns, ","), cmd)

		// replace doVM to only copy fields in column order
		doVM = func(resp *minicli.Response, row []string) map[string]string {
			vm := map[string]string{}
			for _, column := range columns {
				if strings.Contains(column, "host") {
					vm["host"] = resp.Host
					continue
				}

				for i, header := range resp.Header {
					if strings.Contains(column, header) {
						vm[header] = row[i]
					}
				}
			}
			return vm
		}
	}

	// don't record command in history
	cmd = fmt.Sprintf(".record false %v", cmd)

	vms := []map[string]string{}

	for resps := range mm.Run(cmd) {
		for _, resp := range resps.Resp {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			for _, row := range resp.Tabular {
				vms = append(vms, doVM(resp, row))
			}
		}
	}

	return vms
}
