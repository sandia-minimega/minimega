// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import "os"

type Command struct {
	ID int

	// run command in the background and return immediately
	Background bool

	// The command is a slice of strings with the first element being the
	// command, and any other elements as the arguments
	Command []string

	// Files to transfer to the client. Any path given in a file specified
	// here will be rooted at <BASE>/files
	FilesSend []*File

	// Files to transfer back to the master
	FilesRecv []*File

	// PID of the process to signal
	PID int

	// Signal to send to PID
	Signal os.Signal

	// Filter for clients to process commands. Not all fields in a client
	// must be set (wildcards), but all set fields must match for a command
	// to be processed.
	Filter *Client

	// clients that have responded to this command
	CheckedIn []string
}

type File struct {
	Name string
	Perm os.FileMode
	Data []byte
}

func (f File) String() string {
	return f.Name
}

type Response struct {
	// ID counter, must match the corresponding Command
	ID int

	// Names and data for uploaded files
	Files []*File

	// Output from responding command, if any
	Stdout string
	Stderr string
}
