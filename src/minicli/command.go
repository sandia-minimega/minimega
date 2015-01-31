package minicli

type Command struct {
	Pattern  string // the specific pattern that was matched
	Original string // original raw input

	StringArgs map[string]string
	BoolArgs   map[string]bool
	ListArgs   map[string][]string

	Subcommand *Command // parsed command

	Call CLIFunc `json:"-"`
}

func newCommand(pattern []patternItem, input []inputItem) (*Command, int) {
	cmd := Command{
		Pattern:    printPattern(pattern),
		StringArgs: make(map[string]string),
		BoolArgs:   make(map[string]bool),
		ListArgs:   make(map[string][]string)}

outer:
	for i, item := range pattern {
		// We ran out of items before matching all the items in the pattern
		if len(input) <= i {
			// Check if the remaining item is optional
			if item.Type&optionalItems != 0 {
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
	if len(pattern) != len(input) {
		return nil, len(pattern) - 1
	}

	return &cmd, len(pattern) - 1
}
