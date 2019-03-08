// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	log "minilog"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"
	"version"
)

var usageTemplate = `igor is a scheduler for Mega-style clusters.

Usage:

	igor command [arguments]

The commands are:
{{range .}}{{if .Runnable}}
    {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "igor help [command]" for more information about a command.
Use "igor version" for version information.

Additional help topics:
{{range .}}{{if not .Runnable}}
    {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "igor help [topic]" for more information about that topic.

`

var helpTemplate = `{{if .Runnable}}usage: igor {{.UsageLine}}

{{end}}{{.Long | trim}}
`

var documentationTemplate = `/*
{{range .}}{{if .Short}}{{.Short | capitalize}}

{{end}}{{if .Runnable}}Usage:

	igor {{.UsageLine}}

{{end}}{{.Long | trim}}


{{end}}*/
package documentation

`

// tmpl executes the given template text on data, writing the result to w.
func tmpl(w io.Writer, text string, data interface{}) {
	t := template.New("top")
	t.Funcs(template.FuncMap{"trim": strings.TrimSpace, "capitalize": capitalize})
	template.Must(t.Parse(text))
	if err := t.Execute(w, data); err != nil {
		panic(err)
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToTitle(r)) + s[n:]
}

func printUsage(w io.Writer) {
	tmpl(w, usageTemplate, commands)
}

func usage() {
	printUsage(os.Stderr)
	os.Exit(2)
}

// help implements the 'help' command.
func help(args []string) {
	if len(args) == 0 {
		printUsage(os.Stdout)
		// not exit 2: succeeded at 'go help'.
		return
	}
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "usage: go help command\n\nToo many arguments given.\n")
		os.Exit(2) // failed at 'go help'
	}

	arg := args[0]

	// 'go help documentation' generates doc.go.
	if arg == "documentation" {
		buf := new(bytes.Buffer)
		printUsage(buf)
		usage := &Command{Long: buf.String()}
		tmpl(os.Stdout, documentationTemplate, append([]*Command{usage}, commands...))
		return
	}

	for _, cmd := range commands {
		if cmd.Name() == arg {
			tmpl(os.Stdout, helpTemplate, cmd)
			// not exit 2: succeeded at 'go help cmd'.
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Unknown help topic %#q.  Run 'go help'.\n", arg)
	os.Exit(2) // failed at 'go help cmd'
}

func printVersion() {
	fmt.Println("igor", version.Revision, version.Date)
}

// Convert an IP to a PXELinux-compatible string, i.e. 192.0.2.91 -> C000025B
func toPXE(ip net.IP) string {
	ip = ip.To4()
	if ip == nil {
		return ""
	}

	return fmt.Sprintf("%02X%02X%02X%02X", ip[0], ip[1], ip[2], ip[3])
}

// Get the calling user. First try $SUDO_USER, then $USER, then just
// user.Current() as the last resort
func getUser() (*user.User, error) {
	username := os.Getenv("SUDO_USER")
	if username != "" {
		return user.Lookup(username)
	}
	username = os.Getenv("USER")
	if username != "" {
		return user.Lookup(username)
	}
	return user.Current()
}

// Emits a log event stating that a particular action has occurred for a reservation
// and prints out a summary of the reservation.
// NOTE: Stats relies on the order of this data.
//       If you change the order/content please update stats.go
func emitReservationLog(action string, res *Reservation) {
	format := "2006-Jan-2-15:04"
	unsplit := igor.unsplitRange(res.Hosts)
	log.Info("%s	user=%v	resname=%v	id=%v	nodes=%v	start=%v	end=%v	duration=%v\n", action, res.Owner, res.Name, res.ID, unsplit, res.Start.Format(format), res.End.Format(format), res.Duration)
}

// install src into dir, using the hash as the file name. Returns the hash or
// an error.
func install(src, dir, suffix string) (string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// hash the file
	hash := sha1.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", fmt.Errorf("unable to hash file %v: %v", src, err)
	}

	fname := hex.EncodeToString(hash.Sum(nil))

	dst := filepath.Join(dir, fname+suffix)

	// copy the file if it doesn't already exist
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		// need to go back to the beginning of the file since we already read
		// it once to do the hashing
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", err
		}

		f2, err := os.Create(dst)
		if err != nil {
			return "", err
		}
		defer f2.Close()

		if _, err := io.Copy(f2, f); err != nil {
			return "", fmt.Errorf("unable to install %v: %v", src, err)
		}
	} else if err != nil {
		// strange...
		return "", err
	} else {
		log.Info("file with identical hash %v already exists, skipping install of %v.", fname, src)
	}

	return fname, nil
}
