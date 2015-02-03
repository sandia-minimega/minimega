package minicli

import (
	"fmt"
	"strings"
)

type Handler struct {
	HelpShort string   `json:"help_short"` // a brief (one line) help message
	HelpLong  string   `json:"help_long"`  // a descriptive help message
	Patterns  []string `json:"patterns"`   // the pattern that the input should match

	// call back to invoke when the raw input matches the pattern
	Call CLIFunc `json:"-"`

	patternItems [][]patternItem `json:"-"` // the processed patterns, used for matching
}

// compileCommand tests whether the input matches the Handler's pattern and
// builds a command based on the input. If there was no match, the returned
// Command will be nil. The second return value is the number of elements of the
// Handler's pattern that were matched. This can be used to determine which
// handler was the closest match.
func (h *Handler) compile(input []inputItem) (*Command, int) {
	var maxMatchLen int
	for _, pattern := range h.patternItems {
		cmd, matchLen := newCommand(pattern, input, h.Call)
		if cmd != nil {
			return cmd, matchLen
		}

		if matchLen > maxMatchLen {
			maxMatchLen = matchLen
		}
	}

	return nil, maxMatchLen
}

func (h *Handler) suggest(input []inputItem) []string {
	suggestions := []string{}

outer:
	for _, pattern := range h.patternItems {
		var i int
		var item patternItem

		for i, item = range pattern {
			if len(input) == i {
				break
			}

			// Test whether we should keep matching this pattern or not
			switch item.Type {
			case literalString:
				// Consuming the last item from input, check if it's a prefix
				// of this literal string.
				if len(input) == i-1 && strings.HasPrefix(item.Text, input[i].Value) {
					suggestions = append(suggestions, item.Text)
				}
				if input[i].Value != item.Text {
					// Input does not match pattern
					continue outer
				}
			case reqChoice, optChoice:
				for _, choice := range item.Options {
					// Consuming the last item from input, check if it's a
					// prefix of one of the choices.
					if len(input) == i-1 && strings.HasPrefix(choice, input[i].Value) {
						suggestions = append(suggestions, choice)
					}
					// TODO: there's a weird case here where one one option is
					// a prefix of another.
					if choice == input[i].Value {
						continue
					}
				}

				// Invalid choice
				continue outer
			case reqList, optList:
				// Nothing to suggest for lists
				continue outer
			case cmdString:
				// TODO: This is fun, need to recurse to complete the subcommand
			}
		}

		// Finished consuming input items, figure out if the next pattern item
		// has something worth completing.
		switch item.Type {
		case literalString:
			suggestions = append(suggestions, item.Text)
		case reqChoice, optChoice:
			suggestions = append(suggestions, item.Options...)
		}
	}

	return suggestions
}

// Prefix finds the shortest literal string prefix that is shared by all
// patterns associated with this handler. May be the empty string if there is
// no common prefix.
func (h *Handler) Prefix() string {
	prefixes := make([]string, len(h.patternItems))

	for i, patternItems := range h.patternItems {
		literals := make([]string, 0)
		for _, item := range patternItems {
			if item.Type != literalString {
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
	for _, pattern := range h.patternItems {
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
