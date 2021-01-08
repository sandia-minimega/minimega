// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package minicli

type Command struct {
	Original string // original raw input

	StringArgs map[string]string
	BoolArgs   map[string]bool
	ListArgs   map[string][]string

	Subcommand *Command // parsed command

	Call CLIFunc `json:"-"`

	// Record command in history (or not). Checked after the command is
	// executed so the CLIFunc can set Record according to its own logic.
	Record bool

	// Preprocess controls whether the Preprocessor is run for this command or
	// not. Must be set before the Command is executed.
	Preprocess bool

	// Set when the command is intentionally a No-op (the original string
	// contains just a comment). This was added to ensure that lines containing
	// only a comment are recorded in the history.
	Nop bool

	// Source allows developers to keep track of where the command originated
	// from. Setting and using this is entirely up to developers using minicli.
	Source string

	// exact tracks whether the command was formed from prefixes or not, can be
	// used to break ties if there is ambiguity.
	exact bool
}

func newCommand(call CLIFunc) *Command {
	return &Command{
		StringArgs: make(map[string]string),
		BoolArgs:   make(map[string]bool),
		ListArgs:   make(map[string][]string),
		Call:       call,
		exact:      true,
	}
}

// SetSource sets the Source field for a command and all nested subcommands.
func (c *Command) SetSource(source string) {
	c.Source = source

	if c.Subcommand != nil {
		c.Subcommand.SetSource(source)
	}
}

// SetRecord sets the Record field for a command and all nested subcommands.
func (c *Command) SetRecord(record bool) {
	c.Record = record

	if c.Subcommand != nil {
		c.Subcommand.SetRecord(record)
	}
}

// SetPreprocess sets the Preprocess field for a command and all nested subcommands.
func (c *Command) SetPreprocess(v bool) {
	c.Preprocess = v

	if c.Subcommand != nil {
		c.Subcommand.SetPreprocess(v)
	}
}
