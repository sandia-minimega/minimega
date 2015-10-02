// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	log "minilog"
	"strings"
)

type Command struct {
	Pattern  string // the specific pattern that was matched
	Original string // original raw input

	StringArgs map[string]string
	BoolArgs   map[string]bool
	ListArgs   map[string][]string

	Subcommand *Command // parsed command

	Call CLIFunc `json:"-"`

	Record bool // record command in history (or not), default is true

	// Set when the command is intentionally a NoOp (the original string
	// contains just a comment). This was added to ensure that lines containing
	// only a comment are recorded in the history.
	noOp bool

	// Source allows developers to keep track of where the command originated
	// from. Setting and using this is entirely up to developers using minicli.
	Source string
}

func newCommand(pattern patternItems, input *Input, call CLIFunc) (*Command, int, bool) {
	exact := true
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
				return &cmd, i, exact
			}

			return nil, i, exact
		}

		switch {
		case item.Type == literalItem:
			if !strings.HasPrefix(item.Text, input.items[i].Value) {
				return nil, i, exact
			}

			if input.items[i].Value != item.Text {
				log.Debug("matched apropos literal %v : %v", item.Text, input.items[i].Value)
				exact = false
			}
		case item.Type&stringItem != 0:
			cmd.StringArgs[item.Key] = input.items[i].Value
		case item.Type&choiceItem != 0:
			// holds the match
			matched := ""
			for _, choice := range item.Options {
				// Check if item matches as apropos
				if strings.HasPrefix(choice, input.items[i].Value) {
					if choice != input.items[i].Value {
						exact = false
					}
					if matched != "" {
						// We already found a match.
						// Collision.
						return nil, i, exact
					}
					matched = choice
				}
			}

			if matched != "" {
				cmd.BoolArgs[matched] = true
				continue outer
			}

			// Invalid choice
			return nil, i, exact
		case item.Type&listItem != 0:
			res := make([]string, len(input.items)-i)
			for i, v := range input.items[i:] {
				res[i] = v.Value
			}

			cmd.ListArgs[item.Key] = res
			return &cmd, i, exact
		case item.Type == commandItem:
			// Parse the subcommand
			subCmd, err := Compile(input.items[i:].String())
			if err != nil {
				return nil, i, exact
			}

			cmd.Subcommand = subCmd
			return &cmd, i, exact
		}
	}

	// Check whether we consumed all the items from the input or not. If there
	// are extra inputItems, we only matched a prefix of the input. This is
	// problematic as we have commands: "vm info" and "vm info search <terms>"
	// that share the same prefix.
	if len(pattern) != len(input.items) {
		return nil, len(pattern) - 1, exact
	}

	return &cmd, len(pattern) - 1, exact
}
