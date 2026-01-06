// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var backingType = "relative"

var diskCLIHandlers = []minicli.Handler{
	{
		HelpShort: "creates a new disk",
		HelpLong: `
Creates a new qcow2 or raw disk of the specified size.

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
Injects files into a disk.

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

To delete files or directories from an image, specify the delete keyword
before listing the files or directories to delete from the image, separated by
a comma. For example:

	disk inject window7_miniccc.qc2 delete files "Program Files/miniccc.exe"
	disk inject window7_miniccc.qc2 delete files "Users/Administrator/Documents/TestDir"
	disk inject window7_miniccc.qc2 delete files "foo.txt,Temp/bar.zip"
		`,
		Patterns: []string{
			"disk <inject,> <image> files <files like /path/to/src:/path/to/dst>...",
			"disk <inject,> <image> <delete,> files <files like /path/to/src,/path/to/src>...",
			"disk <inject,> <image> <options,> <options> files <files like /path/to/src:/path/to/dst>",
			"disk <inject,> <image> <options,> <options> <delete,> files <files like /path/to/src,/path/to/src>",
		},
		Call: wrapSimpleCLI(cliDiskInject),
	},
	{
		HelpShort: "provides info about a disk",
		HelpLong: `
Provides information about a disk such as format, virtual/actual size, and backing file.
The 'recursive' flag can be set to print out full details for all backing images.`,
		Patterns: []string{"disk info <image> [recursive,]"},
		Call:     wrapSimpleCLI(cliDiskInfo),
	},
	{
		HelpShort: "creates a new disk 'dst' backed by 'image'",
		HelpLong: fmt.Sprintf(`
Creates a new qcow2 image 'dst' backed by 'image'.

Example of taking a snapshot of a disk:

	disk snapshot windows7.qc2 window7_miniccc.qc2

If the destination name is omitted, a name will be randomly generated and the
snapshot will be stored in the 'files' directory. Snapshots are always created
in the 'files' directory.

Users may use paths relative to the 'files' directory or absolute paths for inputs; 
however, the backing path will always be %s to the new image.`, backingType),
		Patterns: []string{"disk snapshot <image> [dst image]"},
		Call:     wrapSimpleCLI(cliDiskSnapshot),
	},
	{
		HelpShort: "rebases the disk onto a different backing image",
		HelpLong: fmt.Sprintf(`
Rebases the image 'image' onto a new backing file 'backing'.
Using 'rebase' will write any differences between the original backing file and the new backing file to 'image'.

		disk rebase myimage.qcow2 base.qcow2

Alternatively, 'set-backing' can be used to change the backing file pointer without any changes to the images.

		disk set-backing myimage.qcow2 base.qcow2

The 'backing' argument can be omitted, causing all backing data to be written to 'image' making it independent.

		disk rebase myimage.qcow2
		
Users may use paths relative to the 'files' directory or absolute paths for inputs; 
however, the backing path will always be %s to the rebased image.`, backingType),
		Patterns: []string{
			"disk <rebase,> <image> [backing file]",
			"disk <set-backing,> <image> [backing file]",
		},
		Call: wrapSimpleCLI(cliDiskRebase),
	},
	{
		HelpShort: "commits the contents of the disk to its backing file",
		HelpLong: `
Commits the contents of 'image' to its backing file. 
'image' is left unchanged, but may be deleted if not needed.
Example of committing:

	disk commit myimage.qcow2`,
		Patterns: []string{"disk commit <image>"},
		Call:     wrapSimpleCLI(cliDiskCommit),
	},
	{
		HelpShort: "resizes a disk",
		HelpLong: `
Changes the size of a disk.
IMPORTANT: Before shrinking an image, ensure changes have been made within the VM's OS to reduce the filesystem.
Similarly, changes in the VM's OS will be required to grow the file system after increasing the disk size.

The size argument is the size in bytes, or using optional suffixes "k"
(kilobyte), "M" (megabyte), "G" (gigabyte), "T" (terabyte).		
It can be given as an absolute value or a relative +/- offset.

Examples:

	disk resize myimage.qcow2 50G
	disk resize myimage.qcow2 +512M`,
		Patterns: []string{"disk resize <image> <size>"},
		Call:     wrapSimpleCLI(cliDiskResize),
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

	delete := strings.Contains(c.Original, " delete files ")

	options := fieldsQuoteEscape("\"", c.StringArgs["options"])
	log.Debug("got options: %v", options)

	var files interface{}
	if _, ok := c.StringArgs["options"]; !ok {
		files = c.ListArgs["files"]
	} else {
		files = c.StringArgs["files"]
	}

	pairs, paths, err := parseFiles(files, delete)
	if err != nil {
		return err
	}

	return diskInject(image, partition, pairs, options, delete, paths)
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
		f, err := os.CreateTemp(*f_iomBase, "snapshot")
		if err != nil {
			return errors.New("could not create a dst image")
		}

		dst = f.Name()
		resp.Response = dst
	} else if !path.IsAbs(dst) {
		dst = path.Join(*f_iomBase, dst)
	}

	log.Debug("destination image: %v", dst)

	return diskSnapshot(image, dst)
}

func cliDiskInfo(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	image := getImage(c)
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

func cliDiskRebase(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	image := getImage(c)

	backingFile, ok := c.StringArgs["backing"]
	if !ok {
		backingFile = ""
	} else if !filepath.IsAbs(backingFile) {
		backingFile = path.Join(*f_iomBase, backingFile)
	}
	_, unsafe := c.BoolArgs["set-backing"]
	return diskRebase(image, backingFile, unsafe)
}

func cliDiskCommit(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	image := getImage(c)
	return diskCommit(image)
}

func cliDiskResize(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	image := getImage(c)
	size := c.StringArgs["size"]
	return diskResize(image, size)
}
