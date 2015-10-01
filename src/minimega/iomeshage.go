// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"iomeshage"
	"meshage"
	"minicli"
	log "minilog"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	IOM_HELPER_WAIT  = time.Duration(100 * time.Millisecond)
	IOM_HELPER_MATCH = "file:"
)

var (
	iom *iomeshage.IOMeshage
)

var ioCLIHandlers = []minicli.Handler{
	{ // file
		HelpShort: "work with files served by minimega",
		HelpLong: `
file allows you to transfer and manage files served by minimega in the
directory set by the -filepath flag (default is 'base'/files).

To list files currently being served, issue the list command with a directory
relative to the served directory:

	file list /foo

Issuing "file list /" will list the contents of the served directory.

Files can be deleted with the delete command:

	file delete /foo

If a directory is given, the directory will be recursively deleted.

Files are transferred using the get command. When a get command is issued, the
node will begin searching for a file matching the path and name within the
mesh. If the file exists, it will be transferred to the requesting node. If
multiple different files exist with the same name, the behavior is undefined.
When a file transfer begins, control will return to minimega while the transfer
completes.

To see files that are currently being transferred, use the status command:

	file status

If a directory is specified, that directory will be recursively transferred to
the node.

You can also supply globs (wildcards) with the * operator. For example:

	file get *.qcow2`,
		Patterns: []string{
			"file <list,> [path]",
			"file <get,> <file>",
			"file <delete,> <file>",
			"file <status,>",
		},
		Call: wrapSimpleCLI(cliFile),
	},
}

func iomeshageInit(node *meshage.Node) {
	var err error
	iom, err = iomeshage.New(*f_iomBase, node)
	if err != nil {
		log.Errorln(err)
		teardown()
	}
}

func cliFile(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	var err error

	if c.BoolArgs["get"] {
		err = iom.Get(c.StringArgs["file"])
	} else if c.BoolArgs["delete"] {
		err = iom.Delete(c.StringArgs["file"])
	} else if c.BoolArgs["status"] {
		transfers := iom.Status()
		resp.Header = []string{"Filename", "Temporary directory", "Completed parts", "Queued"}
		resp.Tabular = [][]string{}

		for _, f := range transfers {
			completed := fmt.Sprintf("%v/%v", len(f.Parts), f.NumParts)
			row := []string{f.Filename, f.Dir, completed, fmt.Sprintf("%v", f.Queued)}
			resp.Tabular = append(resp.Tabular, row)
		}
	} else if c.BoolArgs["list"] {
		path := c.StringArgs["path"]
		if path == "" {
			path = "/"
		}

		resp.Header = []string{"dir", "name", "size"}
		resp.Tabular = [][]string{}

		files, err := iom.List(path)
		if err == nil && files != nil {
			for _, f := range files {
				var dir string
				if f.Dir {
					dir = "<dir>"
				}

				row := []string{dir, f.Name, strconv.FormatInt(f.Size, 10)}
				resp.Tabular = append(resp.Tabular, row)
			}
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

// walk every arg looking for "file:" and calling iomHelper on the suffix.
// Replace the arg with the local file if found.
func iomPreprocessor(c *minicli.Command) (*minicli.Command, error) {
	for k, v := range c.StringArgs {
		if strings.HasPrefix(v, IOM_HELPER_MATCH) {
			file := strings.TrimPrefix(v, IOM_HELPER_MATCH)
			local, err := iomHelper(file)
			if err != nil {
				return nil, err
			}
			log.Debug("iomPreProcessor: %v -> %v", v, local)
			c.StringArgs[k] = local
		}
	}
	for k, v := range c.ListArgs {
		for x, y := range v {
			if strings.HasPrefix(y, IOM_HELPER_MATCH) {
				file := strings.TrimPrefix(y, IOM_HELPER_MATCH)
				local, err := iomHelper(file)
				if err != nil {
					return nil, err
				}
				log.Debug("iomPreProcessor: %v -> %v", y, local)
				c.ListArgs[k][x] = local
			}
		}
	}
	return c, nil
}

// iomHelper supports grabbing files for internal minimega operations. It
// returns the local path of the file or an error if the file doesn't exist or
// could not transfer. iomHelper blocks until all file transfers are completed.
func iomHelper(file string) (string, error) {
	err := iom.Get(file)
	if err != nil {
		return "", err
	}

	// poll until the file transfer is completed
	for {
		var waiting bool
		transfers := iom.Status()
		for _, f := range transfers {
			if strings.Contains(f.Filename, file) {
				log.Debug("iomHelper waiting on %v: %v/%v", f.Filename, len(f.Parts), f.NumParts)
				waiting = true
			}
		}
		if !waiting {
			break
		}
		time.Sleep(IOM_HELPER_WAIT)
	}

	dst := filepath.Join(*f_iomBase, file)
	return dst, nil
}
