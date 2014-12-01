package minicli

type Handler struct {
	Pattern      string //
	patternItems []patternItem
	HelpShort    string
	HelpLong     string
	Call         func(*Command) *Responses
}

// compileCommand tests whether the input matches the Handler's pattern and
// builds a command based on the input. If there was no match, the returned
// Command will be nil. The second return value is the number of elements of the
// Handler's pattern that were matched. This can be used to determine which
// handler was the closest match.
func (h *Handler) compileCommand(input []inputItem) (*Command, int) {
	cmd := Command{
		Pattern:    h.Pattern,
		StringArgs: make(map[string]string),
		BoolArgs:   make(map[string]bool),
		ListArgs:   make(map[string][]string)}

outer:
	for i, pItem := range h.patternItems {
		// We ran out of items before matching all the items in the pattern
		if len(input) <= i {
			// Check if the remaining item is optional
			if pItem.Type == optString || pItem.Type == optList || pItem.Type == optChoice {
				// Matched!
				return &cmd, i
			}

			return nil, i
		}

		switch pItem.Type {
		case literalString:
			if input[i].Value != pItem.Text {
				return nil, i
			}
		case reqString, optString:
			cmd.StringArgs[pItem.Key] = input[i].Value
		case reqChoice, optChoice:
			for _, choice := range pItem.Options {
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

			cmd.ListArgs[pItem.Key] = res
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
