// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/iomeshage"
	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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

	file get *.qcow2
	file delete *.qcow2

The stream command allows users to stream files through the Response. Each part
of the file is returned as a separate response which can then be combined to
form the original file. This command blocks until the stream is complete.`,
		Patterns: []string{
			"file <list,>",
			"file <list,> <path> [recursive,]",
			"file <get,> <file>",
			"file <stream,> <file>",
			"file <delete,> <file>",
			"file <status,>",
		},
		Call: cliFile,
	},
}

func iomeshageStart(node *meshage.Node) error {
	var err error
	iom, err = iomeshage.New(*f_iomBase, node)
	return err
}

func cliFile(c *minicli.Command, respChan chan<- minicli.Responses) {
	fname := c.StringArgs["file"]

	switch {
	case c.BoolArgs["list"]:
		path := c.StringArgs["path"]
		if path == "" {
			path = "/"
		}

		resp := &minicli.Response{Host: hostname}

		resp.Header = []string{"dir", "name", "size", "modified"}
		resp.Tabular = [][]string{}

		recursive := c.BoolArgs["recursive"]

		files, err := iom.List(path, recursive)
		if err != nil {
			respChan <- errResp(err)
			return
		}

		for _, f := range files {
			var dir string
			if f.IsDir() {
				dir = "<dir>"
			}

			row := []string{dir, iom.Rel(f), strconv.FormatInt(f.Size, 10), f.ModTime.Format(time.RFC3339)}
			resp.Tabular = append(resp.Tabular, row)
		}

		respChan <- minicli.Responses{resp}
		return
	case c.BoolArgs["get"]:
		respChan <- errResp(iom.Get(fname))
		return
	case c.BoolArgs["stream"]:
		stream, err := iom.Stream(fname)
		if err != nil {
			respChan <- errResp(err)
			return
		}

		for v := range stream {
			resp := &minicli.Response{
				Host: hostname,
				Data: v,
			}

			respChan <- minicli.Responses{resp}
		}

		return
	case c.BoolArgs["delete"]:
		respChan <- errResp(iom.Delete(fname))
		return
	case c.BoolArgs["status"]:
		resp := &minicli.Response{Host: hostname}

		resp.Header = []string{"filename", "tempdir", "completed", "queued"}
		resp.Tabular = [][]string{}

		for _, f := range iom.Status() {
			completed := fmt.Sprintf("%v/%v", len(f.Parts), f.NumParts)
			row := []string{f.Filename, f.Dir, completed, fmt.Sprintf("%v", f.Queued)}
			resp.Tabular = append(resp.Tabular, row)
		}

		respChan <- minicli.Responses{resp}
		return
	}
}

// iomHelper supports grabbing files for internal minimega operations. It
// returns the local path of the file or an error if the file doesn't exist or
// could not transfer. iomHelper blocks until all file transfers are completed.
// If updatee is provided, it will periodically be sent status update messages
// about file transfer status.
func iomHelper(file, updatee string) (string, error) {
	// remove any weirdness from the filename like '..'
	file = filepath.Clean(file)

	// IOMeshage assumes relative paths so trim off the f_iomBase path prefix
	// if it exists.
	if strings.HasPrefix(file, *f_iomBase) {
		rel, err := filepath.Rel(*f_iomBase, file)
		if err != nil {
			return "", err
		}

		file = rel
	}

	if err := iom.Get(file); err != nil {
		// suppress in-flight error -- we'll just wait as normal
		if err.Error() != "file already in flight" {
			return "", err
		}
	}

	iomWait(file, updatee)

	dst := filepath.Join(*f_iomBase, file)

	info, err := diskInfo(dst)
	if err == nil && info.BackingFile != "" {
		// try to fetch backing image too
		file := filepath.Clean(info.BackingFile)

		if !strings.HasPrefix(file, *f_iomBase) {
			return "", fmt.Errorf("cannot fetch backing image from outside files directory: %v", file)
		}

		file, err = filepath.Rel(*f_iomBase, file)
		if err != nil {
			return "", err
		}

		log.Info("fetching backing image: %v", file)

		if _, err := iomHelper(file, updatee); err != nil {
			return "", fmt.Errorf("failed to fetch backing image %v: %v", file, err)
		}
	}

	return dst, nil
}

// iomWait polls until the file transfer is completed, optionally periodically
// sending status update messages to the updatee if provided
func iomWait(file, updatee string) {
	log.Info("waiting on file: %v", file)

	lastStatus := time.Now()

	meshageStatusLock.RLock()
	period := meshageStatusPeriod
	meshageStatusLock.RUnlock()

outer:
	for {
		for _, f := range iom.Status() {
			if strings.Contains(f.Filename, file) {
				log.Info("iomHelper waiting on %v: %v/%v", f.Filename, len(f.Parts), f.NumParts)

				if updatee != "" && time.Since(lastStatus) >= period {
					var status string

					if len(f.Parts) == f.NumParts {
						status = fmt.Sprintf("merging file %s", f.Filename)
					} else {
						status = fmt.Sprintf("transferring file %s: %f%% complete", f.Filename, float64(len(f.Parts))/float64(f.NumParts)*100.0)
					}

					sendStatusMessage(status, updatee)
					lastStatus = time.Now()
				}

				time.Sleep(IOM_HELPER_WAIT)
				continue outer
			}
		}

		break
	}
}

// a filename completer for goreadline that searches for the file: prefix,
// attempts to find matching files, and returns an array of candidates.
func iomCompleter(last string) []string {
	if !strings.HasPrefix(last, IOM_HELPER_MATCH) {
		return nil
	}

	fileprefix := strings.TrimPrefix(last, IOM_HELPER_MATCH)
	matches := iom.Info(fileprefix + "*")
	log.Debug("got raw matches: %v", matches)

	// we need to clean up matches to collapse directories, unless
	// there is a directory common prefix, in which case we
	// collapse offset by the number of common directories.
	dlcp := lcp(matches)
	didx := strings.LastIndex(dlcp, string(filepath.Separator))
	drel := ""
	if didx > 0 {
		drel = dlcp[:didx]
	}
	log.Debug("dlcp: %v, drel: %v", dlcp, drel)

	if len(fileprefix) < len(drel) {
		r := IOM_HELPER_MATCH + drel + string(filepath.Separator)
		return []string{r, r + "0"} // hack to prevent readline from fastforwarding beyond the directory name
	}

	var finalMatches []string
	for _, v := range matches {
		if strings.Contains(v, "*") {
			continue
		}
		r, err := filepath.Rel(drel, v)
		if err != nil {
			log.Errorln(err)
			return nil
		}
		dir := filepath.Dir(r)
		if dir == "." {
			finalMatches = append(finalMatches, IOM_HELPER_MATCH+v)
			continue
		}

		paths := strings.Split(dir, string(filepath.Separator))
		found := false
		for _, d := range finalMatches {
			if d == paths[0]+string(filepath.Separator) {
				found = true
				break
			}
		}
		if !found {
			finalMatches = append(finalMatches, IOM_HELPER_MATCH+filepath.Join(drel, paths[0])+string(filepath.Separator))
		}
	}

	return finalMatches
}

// a simple longest common prefix function
func lcp(s []string) string {
	var lcp string
	var p int

	if len(s) == 0 {
		return ""
	}

	for {
		var c byte
		for _, v := range s {
			if len(v) <= p {
				return lcp
			}
			if c == 0 {
				c = v[p]
				continue
			}
			if c != v[p] {
				return lcp
			}
		}
		lcp += string(s[0][p])
		p++
	}
}
