package minicli

import "strings"

type Handler struct {
	Pattern   string // the pattern that the input should match
	HelpShort string // a brief (one line) help message
	HelpLong  string // a descriptive help message
	// call back to invoke when the raw input matches the pattern
	Call func(*Command) *Responses

	patternItems []patternItem // the processed pattern, used for matching
}

// compileCommand tests whether the input matches the Handler's pattern and
// builds a command based on the input. If there was no match, the returned
// Command will be nil. The second return value is the number of elements of the
// Handler's pattern that were matched. This can be used to determine which
// handler was the closest match.
func (h *Handler) compileCommand(input []inputItem) (*Command, int) {
	cmd := Command{
		Handler:    *h,
		StringArgs: make(map[string]string),
		BoolArgs:   make(map[string]bool),
		ListArgs:   make(map[string][]string)}

outer:
	for i, item := range h.patternItems {
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
		case cmdString:
			// Parse the subcommand
			subCmd, err := CompileCommand(printInput(input[i:]))
			if err != nil {
				return nil, i
			}

			cmd.Subcommand = subCmd
		}
	}

	return &cmd, len(h.Pattern) - 1
}

func (h *Handler) literalPrefix() string {
	literals := make([]string, 0)
	for _, item := range h.patternItems {
		if item.Type != literalString {
			break
		}

		literals = append(literals, item.Text)
	}

	return strings.Join(literals, " ")
}
