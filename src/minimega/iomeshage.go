package main

import (
	"bytes"
	"fmt"
	"iomeshage"
	"meshage"
	log "minilog"
	"text/tabwriter"
)

var (
	iom *iomeshage.IOMeshage
)

func iomeshageInit(node *meshage.Node) {
	var err error
	iom, err = iomeshage.New(*f_iomBase, node, true)
	if err != nil {
		log.Errorln(err)
		teardown()
	}
}

func cliFile(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 1: // list, status
		switch c.Args[0] {
		case "list":
			l, err := iomList("/")
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
			return cliResponse{
				Response: l,
			}
		case "status":
			s := iomStatus()
			return cliResponse{
				Response: s,
			}
		default:
			return cliResponse{
				Error: "malformed command",
			}
		}
	case 2: // list, delete, get
		switch c.Args[0] {
		case "list":
			l, err := iomList(c.Args[1])
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
			return cliResponse{
				Response: l,
			}
		case "delete":
			err := iom.Delete(c.Args[1])
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
			return cliResponse{}
		case "get":
			err := iom.Get(c.Args[1])
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
			return cliResponse{}
		default:
			return cliResponse{
				Error: "malformed command",
			}
		}
	default:
		return cliResponse{
			Error: "file takes at least one argument",
		}
	}
}

func iomList(dir string) (string, error) {
	files, err := iom.List(dir)
	if err != nil {
		return "", err
	}
	if files == nil {
		return "", nil
	}

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	for _, f := range files {
		n := f.Name
		if f.Dir {
			n += " <dir>"
		}
		fmt.Fprintf(w, "%v\t%v\n", n, f.Size)
	}
	w.Flush()
	return o.String(), nil
}

func iomStatus() string {
	transfers := iom.Status()
	if transfers == nil {
		return ""
	}
	return fmt.Sprintln("%v", transfers)
}
