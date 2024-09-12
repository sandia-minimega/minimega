// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var diskCLIHandlers = []minicli.Handler{
	{ // disk
		HelpShort: "creates a new image 'dst' backed by 'image",
		HelpLong: `
Example of taking a snapshot of a disk:

	disk snapshot windows7.qc2 window7_miniccc.qc2

If the destination name is omitted, a name will be randomly generated and the
snapshot will be stored in the 'files' directory. Snapshots are always created
in the 'files' directory.

Disk image paths are always relative to the 'files' directory. Users may also
use absolute paths if desired. The backing images for snapshots should always
be in the files directory.`,
		Patterns: []string{"disk snapshot <image> [dst image]"},
		Call:     wrapSimpleCLI(cliDiskSnapshot),
	},
	{
		HelpShort: "creates a new disk",
		HelpLong: `
Example of creating a new disk:

	disk create qcow2 foo.qcow2 100G

The size argument is the size in bytes, or using optional suffixes "k"
(kilobyte), "M" (megabyte), "G" (gigabyte), "T" (terabyte).		
		`,
		Patterns: []string{"disk create <qcow2,raw> <image name> <size>"},
		Call:     wrapSimpleCLI(cliDiskCreate),
	},
	{
		HelpShort: "injects files into a disk",
		HelpLong: `
To inject files into an image:

	disk inject window7_miniccc.qc2 files "miniccc":"Program Files/miniccc"

Each argument after the image should be a source and destination pair,
separated by a ':'. If the file paths contain spaces, use double quotes.
Optionally, you may specify a partition (partition 1 will be used by default):

	disk inject window7_miniccc.qc2:2 files "miniccc":"Program Files/miniccc"

You may also specify that there is no partition on the disk, if your filesystem
was directly written to the disk (this is highly unusual):

	disk inject partitionless_disk.qc2:none files /miniccc:/miniccc

You can optionally specify mount arguments to use with inject. Multiple options
should be quoted. For example:

	disk inject foo.qcow2 options "-t fat -o offset=100" files foo:bar		
		`,
		Patterns: []string{
			"disk inject <image> files <files like /path/to/src:/path/to/dst>...",
			"disk inject <image> options <options> files <files like /path/to/src:/path/to/dst>...",
		},
		Call: wrapSimpleCLI(cliDiskInject),
	},
	{
		HelpShort: "provides info about a disk",
		HelpLong:  "Provides information about a disk such as format, size, and backing file.",
		Patterns:  []string{"disk info <image> [recursive,]"},
		Call:      wrapSimpleCLI(cliDiskInfo),
	},
}

func getImage(c *minicli.Command) string {
	image := filepath.Clean(c.StringArgs["image"])

	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(image) {
		image = path.Join(*f_iomBase, image)
	}
	return image
}

// parseInjectPairs parses a list of strings containing src:dst pairs into a
// map of where the dst is the key and src is the value. We build the map this
// way so that one source file can be written to multiple destinations and so
// that we can detect and return an error if the user tries to inject two files
// with the same destination.
func parseInjectPairs(files []string) (map[string]string, error) {
	pairs := map[string]string{}

	// parse inject pairs
	for _, arg := range files {
		parts := strings.Split(arg, ":")
		if len(parts) != 2 {
			return nil, errors.New("malformed command; expected src:dst pairs")
		}

		if pairs[parts[1]] != "" {
			return nil, fmt.Errorf("destination appears twice: `%v`", parts[1])
		}

		pairs[parts[1]] = parts[0]
		log.Debug("inject pair: %v, %v", parts[0], parts[1])
	}

	return pairs, nil
}

func cliDiskInject(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	image := getImage(c)
	var partition string

	if strings.Contains(image, ":") {
		parts := strings.Split(image, ":")
		if len(parts) != 2 {
			return errors.New("found way too many ':'s, expected <path/to/image>:<partition>")
		}

		image, partition = parts[0], parts[1]
	}

	options := fieldsQuoteEscape("\"", c.StringArgs["options"])
	log.Debug("got options: %v", options)

	pairs, err := parseInjectPairs(c.ListArgs["files"])
	if err != nil {
		return err
	}

	return diskInject(image, partition, pairs, options)
}

func cliDiskCreate(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	image := getImage(c)
	size := c.StringArgs["size"]
	format := "raw"
	if _, ok := c.BoolArgs["qcow2"]; ok {
		format = "qcow2"
	}

	return diskCreate(format, image, size)
}

func cliDiskSnapshot(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	image := getImage(c)
	dst := c.StringArgs["dst"]

	if dst == "" {
		f, err := ioutil.TempFile(*f_iomBase, "snapshot")
		if err != nil {
			return errors.New("could not create a dst image")
		}

		dst = f.Name()
		resp.Response = dst
	} else if strings.Contains(dst, "/") {
		return errors.New("dst image must filename without path")
	} else {
		dst = path.Join(*f_iomBase, dst)
	}

	log.Debug("destination image: %v", dst)

	return diskSnapshot(image, dst)
}

func cliDiskInfo(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	image := filepath.Clean(c.StringArgs["image"])
	resp.Header = []string{"image", "format", "virtualsize", "disksize", "backingfile", "inuse"}

	infos := []DiskInfo{}
	var err error

	if c.BoolArgs["recursive"] {
		infos, err = diskChainInfo(image)
	} else {
		var info DiskInfo
		info, err = diskInfo(image)
		infos = append(infos, info)
	}

	if err != nil {
		return err
	}

	for _, info := range infos {
		resp.Tabular = append(resp.Tabular, []string{
			info.Name, info.Format, humanReadableBytes(info.VirtualSize), humanReadableBytes(info.DiskSize), info.BackingFile, strconv.FormatBool(info.InUse),
		})
	}

	return nil
}
