// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"sort"
	"strconv"
)

func sortVMs(vms []map[string]string) {
	sort.Slice(vms, func(i, j int) bool {
		// first, sort by host
		h := vms[i]["host"]
		h2 := vms[j]["host"]

		if h != h2 {
			return h < h2
		}

		// then sort by IDs, if present
		id, err := strconv.Atoi(vms[i]["id"])
		id2, err2 := strconv.Atoi(vms[j]["id"])

		if err == nil && err2 == nil && id != id2 {
			return id < id2
		}

		// lastly, by names (hopefully present)
		return vms[i]["name"] < vms[j]["name"]
	})
}
