// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io"
	log "minilog"
	"net"
	"os"
	"os/user"
	"ranges"
	"strings"
	"text/template"
	"time"
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
	s := fmt.Sprintf("%02X%02X%02X%02X", ip[12], ip[13], ip[14], ip[15])
	return s
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
func emitReservationLog(action string, res Reservation) {
	format := "2006-Jan-2-15:04"
	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)
	unsplit, _ := rnge.UnsplitRange(res.Hosts)
	log.Info("%s	user=%v	resname=%v	nodes=%v	start=%v	end=%v	duration=%v\n", action, res.Owner, res.ResName, unsplit, time.Unix(res.StartTime, 0).Format(format), time.Unix(res.EndTime, 0).Format(format), res.Duration)
}
