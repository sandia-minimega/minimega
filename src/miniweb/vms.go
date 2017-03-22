// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"strings"
)

func vmInfo(name string, columns []string) []map[string]string {
	cmd := "vm info"
	if name != "" {
		// TODO: quotes?
		cmd = fmt.Sprintf(".filter name=%v %v", name, cmd)
	}

	if len(columns) != 0 {
		// TODO: quotes?
		cmd = fmt.Sprintf(".columns %v %v", strings.Join(columns, ","), cmd)
	}

	res := []map[string]string{}

	for resps := range mm.Run(cmd) {
		for _, resp := range resps.Resp {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			for _, row := range resp.Tabular {
				vm := map[string]string{}
				for _, column := range columns {
					if column == "host" {
						vm["host"] = resp.Host
						continue
					}

					for i, header := range resp.Header {
						if column == header {
							vm[header] = row[i]
						}
					}
				}

				res = append(res, vm)
			}
		}
	}

	if len(res) == 0 {
		log.Errorln("no vms")
		return nil
	}

	if name != "" && len(res) > 1 {
		log.Errorln("lots of vms")
	}

	return res
}
