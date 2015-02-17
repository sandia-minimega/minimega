// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

type Command struct {
	ID int

	// run command in the background and return immediately
	Background bool

	// The command is a slice of strings with the first element being the
	// command, and any other elements as the arguments
	Command []string

	// Files to transfer to the client if type == COMMAND_EXEC | COMMAND_FILE_SEND
	// Any path given in a file specified here will be rooted at <BASE>/files
	FilesSend []string

	// Files to transfer back to the master if type == COMMAND_EXEC | COMMAND_FILE_RECV
	FilesRecv []string

	// Filter for clients to process commands. Not all fields in a client
	// must be set (wildcards), but all set fields must match for a command
	// to be processed.
	Filter *Client

	// clients that have responded to this command
	// leave this private as we don't want to bother sending this
	// downstream
	CheckedIn []string
}

type Response struct {
	// ID counter, must match the corresponding Command
	ID int

	// Names and data for uploaded files
	Files map[string][]byte

	// Output from responding command, if any
	Stdout string
}

