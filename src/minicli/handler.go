// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	"fmt"
	log "minilog"
	"strings"
)

type Handler struct {
	HelpShort string   `json:"help_short"` // a brief (one line) help message
	HelpLong  string   `json:"help_long"`  // a descriptive help message
	Patterns  []string `json:"patterns"`   // the pattern that the input should match

	// prefix shared by all patterns, automatically populated when
	SharedPrefix string `json:"shared_prefix"`

	// call back to invoke when the raw input matches the pattern
	Call CLIFunc `json:"-"`

	// the processed patterns, will be automatically populated when the command
	// is registered
	PatternItems [][]PatternItem `json:"parsed_patterns"`

	// Suggest provides suggestions for variable completion. For example, the
	// `vm stop` command might provide a listing of the currently running VM
	// names if the user tries to tab complete the "target". The function takes
	// three arguments: the raw input string, the variable name (e.g.
	// "target"), and the user's input for the variable so far.
	Suggest SuggestFunc `json:"-"`
}

// compileCommand tests whether the input matches the Handler's pattern and
// builds a command based on the input. If there was no match, the returned
// Command will be nil. The second return value is the number of elements of the
// Handler's pattern that were matched. This can be used to determine which
// handler was the closest match. The third return value is true if there
// pattern is an exact match, not an apropos match.
func (h *Handler) compile(input *Input) (*Command, int, bool) {
	var maxMatchLen int
	var cmd *Command
	var matchLen int
	var exact bool

	for i, pattern := range h.PatternItems {
		cmd, matchLen, exact = newCommand(pattern, input, h.Call)
		if cmd != nil {
			// patch up patterns from original pattern strings
			cmd.Pattern = h.Patterns[i]
			return cmd, matchLen, exact
		}

		if matchLen > maxMatchLen {
			maxMatchLen = matchLen
		}
	}

	return nil, maxMatchLen, false
}

func (h *Handler) parsePatterns() error {
	for _, pattern := range h.Patterns {
		items, err := lexPattern(pattern)
		if err != nil {
			return err
		}

		h.PatternItems = append(h.PatternItems, items)
	}

	return nil
}

func (h *Handler) suggest(raw string, input *Input) []string {
	suggestions := []string{}

outer:
	for _, pattern := range h.PatternItems {
		var i int
		var item PatternItem

		for i, item = range pattern {
			// We ran out of input items before pattern items, make suggestion
			// based on the next pattern item
			if len(input.items) == i {
				break
			}

			val := input.items[i].Value

			// Test whether we should keep matching this pattern or not
			switch {
			case item.Type == literalItem:
				if !strings.HasPrefix(item.Text, val) {
					continue outer
				}
			case item.Type&choiceItem != 0:
				var found bool
				for _, choice := range item.Options {
					found = found || strings.HasPrefix(choice, val)
				}

				// Invalid choice
				if !found {
					continue outer
				}
			case item.Type&listItem != 0:
				// Nothing to suggest for lists
				continue outer
			case item.Type == commandItem:
				// This is fun, need to recurse to complete the subcommand
				log.Debug("recursing to find suggestions for %q", input.items[i:])
				suggestions = append(suggestions, suggest(raw, &Input{
					Original: input.Original,
					items:    input.items[i:],
				})...)
			}

			// Before proceeding to the next pattern item, check whether the
			// input is ``complete'' or not -- based on whether it is followed
			// by a space. If the input is not complete, and we are consuming
			// the last input element, we should suggest for the current
			// pattern item and not the next one.
			if len(input.items) == i+1 && !strings.HasSuffix(input.Original, " ") {
				break
			}
		}

		// Don't make suggestions if we have consumed the whole pattern
		if len(input.items) == len(pattern) && strings.HasSuffix(input.Original, " ") {
			continue
		}

		// Skip over patterns that are shorter than the input unless they have
		// a nested subcommand
		if len(input.items) > len(pattern) && item.Type != commandItem {
			continue
		}

		// Finished consuming input items, figure out if the next pattern item
		// has something worth completing.
		switch item.Type {
		case literalItem:
			suggestions = append(suggestions, item.Text)
		case choiceItem, choiceItem | optionalItem:
			for _, choice := range item.Options {
				if i >= len(input.items) || strings.HasPrefix(choice, input.items[i].Value) {
					suggestions = append(suggestions, choice)
				}
			}
		case stringItem, listItem, stringItem | optionalItem, listItem | optionalItem:
			if h.Suggest != nil {
				var prefix string
				if i < len(input.items) {
					prefix = input.items[i].Value
				}
				suggestions = append(suggestions, h.Suggest(raw, item.Key, prefix)...)
			}
		case commandItem:
			log.Debug("recursing to find suggestions for %q", input.items[i:])
			suggestions = append(suggestions, suggest(raw, &Input{
				Original: input.Original,
				items:    input.items[i:],
			})...)
		}
	}

	return suggestions
}

// findPrefix finds the shortest literal string prefix that is shared by all
// patterns associated with this handler. May be the empty string if there is
// no common prefix.
func (h *Handler) findPrefix() string {
	prefixes := make([]string, len(h.PatternItems))

	for i, patternItems := range h.PatternItems {
		literals := make([]string, 0)
		for _, item := range patternItems {
			if item.Type != literalItem {
				break
			}

			literals = append(literals, item.Text)
		}

		prefix := strings.Join(literals, " ")
		if len(prefix) == 0 {
			return ""
		}

		prefixes[i] = prefix
	}

	shared := prefixes[0]
	for i := 1; i < len(prefixes) && len(shared) > 0; i++ {
		prefix := prefixes[i]

		var j int
		for ; j < len(shared) && j < len(prefix) && shared[j] == prefix[j]; j++ {
			// Do nothing... just increment j
		}
		shared = shared[:j]
	}

	return strings.TrimSpace(shared)
}

func (h *Handler) helpShort() string {
	return h.HelpShort
}

func (h *Handler) helpLong() string {
	res := "Usage:\n"
	for _, pattern := range h.PatternItems {
		res += fmt.Sprintf("\t%s\n", patternItems(pattern))
	}
	res += "\n"
	// Fallback on HelpShort if there's no HelpLong
	if len(h.HelpLong) > 0 {
		res += h.HelpLong
	} else {
		res += h.HelpShort
	}

	return res
}
