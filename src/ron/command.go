// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"fmt"
	log "minilog"
	"os"
	"strings"
)

type Filter struct {
	UUID     string
	Hostname string
	Arch     string
	OS       string
	MAC      string
	IP       string
	Tags     map[string]string
}

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

	// PID of the process to signal, -1 signals all processes
	PID int

	// KillAll kills all processes by name
	KillAll string

	// Level adjusts the minilog level
	Level *log.Level

	// Filter for clients to process commands. Not all fields in a client
	// must be set (wildcards), but all set fields must match for a command
	// to be processed.
	Filter *Filter

	// clients that have responded to this command
	CheckedIn []string

	// Prefix is an optional field that can be used to track commands. It is
	// not used by the server or client.
	Prefix string

	// plumber connections
	Stdin  string
	Stdout string
	Stderr string
}

type File struct {
	Name string
	Perm os.FileMode
	Data []byte
}

func (f File) String() string {
	return f.Name
}

func (f *File) Copy() *File {
	f2 := &File{
		Name: f.Name,
		Perm: f.Perm,
	}
	f2.Data = append(f2.Data, f.Data...)

	return f2
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

func (f *Filter) String() string {
	if f == nil {
		return ""
	}

	var res []string
	if f.UUID != "" {
		res = append(res, "uuid="+f.UUID)
	}
	if f.Hostname != "" {
		res = append(res, "hostname="+f.Hostname)
	}
	if f.Arch != "" {
		res = append(res, "arch="+f.Arch)
	}
	if f.OS != "" {
		res = append(res, "os="+f.OS)
	}
	if f.IP != "" {
		res = append(res, "ip="+f.IP)
	}
	if f.MAC != "" {
		res = append(res, "mac="+f.MAC)
	}
	for k, v := range f.Tags {
		res = append(res, fmt.Sprintf("%v=%v", k, v))
	}

	return strings.Join(res, " && ")
}

// Creates a copy of c.
func (c *Command) Copy() *Command {
	c2 := &Command{
		ID:         c.ID,
		Background: c.Background,
		PID:        c.PID,
		KillAll:    c.KillAll,
		Prefix:     c.Prefix,
		Stdin:      c.Stdin,
		Stdout:     c.Stdout,
		Stderr:     c.Stderr,
	}

	// make deep copies
	c2.Command = append(c2.Command, c.Command...)
	c2.CheckedIn = append(c2.CheckedIn, c.CheckedIn...)

	for _, f := range c.FilesSend {
		c2.FilesSend = append(c2.FilesSend, f.Copy())
	}
	for _, f := range c.FilesRecv {
		c2.FilesRecv = append(c2.FilesRecv, f.Copy())
	}

	if c.Filter != nil {
		c2.Filter = new(Filter)
		*c2.Filter = *c.Filter
	}
	if c.Level != nil {
		c2.Level = new(log.Level)
		*c2.Level = *c.Level
	}

	return c2
}
