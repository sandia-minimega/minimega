// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"sort"
)

// sort and walk the api, emitting markdown for each entry
func docGen() {
	var keys []string
	// TODO
	//for k, _ := range cliCommands {
	//	keys = append(keys, k)
	//}
	sort.Strings(keys)

	fmt.Println("# minimega API")

	for _, k := range keys {
		fmt.Printf("<h2 id=%v>%v</h2>\n", k, k)
		//fmt.Println(cliCommands[k].Helplong)
	}
}
