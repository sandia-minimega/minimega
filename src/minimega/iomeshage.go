package main

import (
	"bytes"
	"fmt"
	"iomeshage"
	"meshage"
	"minicli"
	log "minilog"
	"strconv"
	"text/tabwriter"
)

var (
	iom *iomeshage.IOMeshage
)

var ioCLIHandlers = []minicli.Handler{
	{ // file
		HelpShort: "work with files served by minimega",
		HelpLong: `
File allows you to transfer and manage files served by minimega in the
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

	file status`,
		Patterns: []string{
			"file <list,> [path]",
			"file <get,> <file>",
			"file <delete,> <file>",
			"file <status,>",
		},
		Call: wrapSimpleCLI(cliFile),
	},
}

func init() {
	registerHandlers("io", ioCLIHandlers)
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
		resp.Response = iomStatus()
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

func iomStatus() string {
	transfers := iom.Status()
	if transfers == nil {
		return ""
	}

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "Filename\tTemporary directory\tCompleted parts\n")
	for _, f := range transfers {
		fmt.Fprintf(w, "%v\t%v\t%v/%v\n", f.Filename, f.Dir, len(f.Parts), f.NumParts)
	}
	w.Flush()
	return o.String()
}
