package minicli

import "strings"

type Handler struct {
	HelpShort string   // a brief (one line) help message
	HelpLong  string   // a descriptive help message
	Patterns  []string // the pattern that the input should match
	// call back to invoke when the raw input matches the pattern
	Call func(*Command) Responses
	// whether the command should be recorded in the command history or not.
	// This allows us to differentiate between commands to that can be used to
	// rebuild the experiment and commands that display information about the
	// current experiment. Note: all lines will be recorded in readline's
	// history, regardless of the value of the record flag.
	Record bool

	patternItems [][]patternItem // the processed patterns, used for matching
}

// compileCommand tests whether the input matches the Handler's pattern and
// builds a command based on the input. If there was no match, the returned
// Command will be nil. The second return value is the number of elements of the
// Handler's pattern that were matched. This can be used to determine which
// handler was the closest match.
func (h *Handler) compileCommand(input []inputItem) (*Command, int) {
	var maxMatchLen int
	for i := range h.patternItems {
		cmd, matchLen := h.compileCommandWithPattern(i, input)
		if cmd != nil {
			return cmd, matchLen
		}

		if matchLen > maxMatchLen {
			maxMatchLen = matchLen
		}
	}

	return nil, maxMatchLen
}

// compileCommandWithPattern attempts to compile a command using the pattern at index idx.
func (h *Handler) compileCommandWithPattern(idx int, input []inputItem) (*Command, int) {
	cmd := Command{
		Handler:    *h,
		Pattern:    h.Patterns[idx],
		StringArgs: make(map[string]string),
		BoolArgs:   make(map[string]bool),
		ListArgs:   make(map[string][]string)}

outer:
	for i, item := range h.patternItems[idx] {
		// We ran out of items before matching all the items in the pattern
		if len(input) <= i {
			// Check if the remaining item is optional
			if item.Type == optString || item.Type == optList || item.Type == optChoice {
				// Matched!
				return &cmd, i
			}

			return nil, i
		}

		switch item.Type {
		case literalString:
			if input[i].Value != item.Text {
				return nil, i
			}
		case reqString, optString:
			cmd.StringArgs[item.Key] = input[i].Value
		case reqChoice, optChoice:
			for _, choice := range item.Options {
				if choice == input[i].Value {
					cmd.BoolArgs[choice] = true
					continue outer
				}
			}

			// Invalid choice
			return nil, i
		case reqList, optList:
			res := make([]string, len(input)-i)
			for i, v := range input[i:] {
				res[i] = v.Value
			}

			cmd.ListArgs[item.Key] = res
			return &cmd, i
		case cmdString:
			// Parse the subcommand
			subCmd, err := CompileCommand(printInput(input[i:]))
			if err != nil {
				return nil, i
			}

			cmd.Subcommand = subCmd
			return &cmd, i
		}
	}

	// Check whether we consumed all the items from the input or not. If there
	// are extra inputItems, we only matched a prefix of the input. This is
	// problematic as we have commands: "vm info" and "vm info search <terms>"
	// that share the same prefix.
	if len(h.patternItems[idx]) != len(input) {
		return nil, len(h.patternItems[idx]) - 1
	}

	return &cmd, len(h.patternItems[idx]) - 1
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
	for _, pattern := range h.Patterns {
		res += "\t" + pattern + "\n"
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
