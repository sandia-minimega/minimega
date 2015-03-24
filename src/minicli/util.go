// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	"bytes"
	"fmt"
	"sort"
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

func printHelpShort(helpShort map[string]string) string {
	var sortedNames []string
	for c, _ := range helpShort {
		sortedNames = append(sortedNames, c)
	}
	sort.Strings(sortedNames)

	res := "Display help on a command. Here is a list of commands:\n"
	w := new(tabwriter.Writer)
	buf := bytes.NewBufferString(res)
	w.Init(buf, 0, 8, 0, '\t', 0)
	for _, c := range sortedNames {
		fmt.Fprintln(w, c, "\t", ":\t", helpShort[c], "\t")
	}
	w.Flush()

	return buf.String()
}

// closestMatch processes the input items and finds the closest match. For
// successful matches, the returned command will be non-nil. Otherwise, the
// handler will contain the closest match if there is at least one input item.
func closestMatch(input *Input) (*Handler, *Command) {
	// Keep track of what was the closest
	var closestHandler *Handler
	var longestMatch int
	var matchedHandler *Handler
	var matchedCmd *Command

	for _, h := range handlers {
		cmd, matchLen := h.compile(input)
		if cmd != nil {
			if matchedHandler != nil { // multiple apropos matches
				return nil, nil
			}
			matchedHandler = h
			matchedCmd = cmd
		}

		if matchLen > longestMatch {
			closestHandler = h
			longestMatch = matchLen
		}
	}

	// return the handler/cmd on perfect or apropos matches
	if matchedHandler != nil {
		return matchedHandler, matchedCmd
	}

	if longestMatch == 0 {
		return nil, nil
	}

	return closestHandler, nil
}
