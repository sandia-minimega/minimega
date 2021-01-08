// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package minicli

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
)

// identicalHelp checks whether the short and long help are identical for all
// handlers in the provided slice.
func identicalHelp(handlers []*Handler) bool {
	for i := 1; i < len(handlers); i++ {
		if handlers[i-1].HelpShort != handlers[i].HelpShort ||
			handlers[i-1].HelpLong != handlers[i].HelpLong {
			return false
		}
	}

	return true
}

func printHelpShort(handlers []*Handler) string {
	sort.Slice(handlers, func(i, j int) bool {
		return strings.Compare(handlers[i].SharedPrefix, handlers[j].SharedPrefix) <= 0
	})
	keys := []string{}
	vals := map[string]string{}
	for _, h := range handlers {
		for _, p := range h.Patterns {
			keys = append(keys, p)
			vals[p] = h.helpShort()
		}
	}
	sort.Strings(keys)

	res := "Display help on a command. Here is a list of commands:\n"
	w := new(tabwriter.Writer)
	buf := bytes.NewBufferString(res)
	w.Init(buf, 0, 8, 0, '\t', 0)
	for _, v := range keys {
		fmt.Fprintln(w, v, "\t", ":\t", vals[v], "\t")
	}
	w.Flush()

	return buf.String()
}
