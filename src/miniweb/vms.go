// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"sort"
	"strconv"
)

func vmInfo(columns, filters []string) []map[string]string {
	return runTabular("vm info", columns, filters)
}

func vmTop(columns, filters []string) []map[string]string {
	return runTabular("vm top", columns, filters)
}

func sortVMs(vms []map[string]string) {
	sort.Slice(vms, func(i, j int) bool {
		h := vms[i]["host"]
		h2 := vms[j]["host"]

		if h == h2 {
			// used IDs, if present
			id, err := strconv.Atoi(vms[i]["id"])
			id2, err2 := strconv.Atoi(vms[j]["id"])

			if err == nil && err2 == nil {
				return id < id2
			}

			// fallback on names (hopefully present)
			return vms[i]["name"] < vms[j]["name"]
		}

		return h < h2
	})
}
