// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

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

func newCommand(pattern patternItems, input *Input, call CLIFunc) (*Command, int) {
	cmd := Command{
		Pattern:    pattern.String(),
		Original:   input.Original,
		StringArgs: make(map[string]string),
		BoolArgs:   make(map[string]bool),
		ListArgs:   make(map[string][]string),
		Call:       call}

outer:
	for i, item := range pattern {
		// We ran out of items before matching all the items in the pattern
		if len(input.items) <= i {
			// Check if the remaining item is optional
			if item.Type&optionalItem != 0 {
				// Matched!
				return &cmd, i
			}

			return nil, i
		}

		switch {
		case item.Type == literalItem:
			if input.items[i].Value != item.Text {
				return nil, i
			}
		case item.Type&stringItem != 0:
			cmd.StringArgs[item.Key] = input.items[i].Value
		case item.Type&choiceItem != 0:
			for _, choice := range item.Options {
				if choice == input.items[i].Value {
					cmd.BoolArgs[choice] = true
					continue outer
				}
			}

			// Invalid choice
			return nil, i
		case item.Type&listItem != 0:
			res := make([]string, len(input.items)-i)
			for i, v := range input.items[i:] {
				res[i] = v.Value
			}

			cmd.ListArgs[item.Key] = res
			return &cmd, i
		case item.Type == commandItem:
			// Parse the subcommand
			subCmd, err := CompileCommand(input.items[i:].String())
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
	if len(pattern) != len(input.items) {
		return nil, len(pattern) - 1
	}

	return &cmd, len(pattern) - 1
}
