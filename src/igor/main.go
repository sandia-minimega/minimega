// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// igor is a simple command line tool for managing reservations of nodes in a
// cluster. It also will configure the pxeboot environment for booting kernels
// and initrds for cluster nodes.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"
)

var configpath = flag.String("config", "/etc/igor.conf", "Path to configuration file")
var igorConfig Config

// The configuration of the system
type Config struct {
	TFTPRoot   string
	Prefix     string
	Start      int
	End        int
	Rackwidth  int
	Rackheight int
}

var Reservations map[string][]string // maps a reservation name to a slice of node names

// A Command is an implementation of a go command
// like go build or go fix.
type Command struct {
	// Run runs the command.
	// The args are the arguments after the command name.
	Run func(cmd *Command, args []string)

	// UsageLine is the one-line usage message.
	// The first word in the line is taken to be the command name.
	UsageLine string

	// Short is the short description shown in the 'go help' output.
	Short string

	// Long is the long message shown in the 'go help <this-command>' output.
	Long string

	// Flag is a set of flags specific to this command.
	Flag flag.FlagSet

	// CustomFlags indicates that the command will do its own
	// flag parsing.
	CustomFlags bool
}

// Name returns the command's name: the first word in the usage line.
func (c *Command) Name() string {
	name := c.UsageLine
	i := strings.Index(name, " ")
	if i >= 0 {
		name = name[:i]
	}
	return name
}

func (c *Command) Usage() {
	fmt.Fprintf(os.Stderr, "usage: %s\n\n", c.UsageLine)
	fmt.Fprintf(os.Stderr, "%s\n", strings.TrimSpace(c.Long))
	os.Exit(2)
}

// Runnable reports whether the command can be run; otherwise
// it is a documentation pseudo-command such as importpath.
func (c *Command) Runnable() bool {
	return c.Run != nil
}

// Commands lists the available commands and help topics.
// The order here is the order in which they are printed by 'go help'.
var commands = []*Command{
	cmdAddtime,
	cmdDel,
	cmdShow,
	cmdSub,
}

var exitStatus = 0
var exitMu sync.Mutex

func setExitStatus(n int) {
	exitMu.Lock()
	if exitStatus < n {
		exitStatus = n
	}
	exitMu.Unlock()
}

func readConfig(path string) (c Config) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		fatalf("Couldn't read config file: %v", err)
	}

	err = json.Unmarshal(b, &c)
	if err != nil {
		fatalf("Couldn't parse json: %v", err)
	}
	return
}

// Read the reservations, delete any that are too old.
func cleanOld() {
	path := filepath.Join(igorConfig.TFTPRoot, "/igor/reservations.json")
	resdb, err := os.OpenFile(path, os.O_RDWR, 664)
	if err != nil {
		fatalf("failed to open reservations file: %v", err)
	}
	defer resdb.Close()
	// We lock to make sure it doesn't change from under us
	// NOTE: not locking for now, haven't decided how important it is
	//err = syscall.Flock(int(resdb.Fd()), syscall.LOCK_EX)
	//defer syscall.Flock(int(resdb.Fd()), syscall.LOCK_UN)	// this will unlock it later
	reservations := getReservations(resdb)

	now := time.Now().Unix()

	for _, r := range reservations {
		if r.Expiration < now {
			deleteReservation(false, []string{r.ResName})
		}
	}
}

func main() {
	flag.Usage = usage
	flag.Parse()
	log.SetFlags(0)

	args := flag.Args()
	if len(args) < 1 {
		usage()
	}

	if args[0] == "help" {
		help(args[1:])
		return
	}

	igorConfig = readConfig(*configpath)

	// Diagnose common mistake: GOPATH==GOROOT.
	// This setting is equivalent to not setting GOPATH at all,
	// which is not what most people want when they do it.
	if gopath := os.Getenv("GOPATH"); gopath == runtime.GOROOT() {
		fmt.Fprintf(os.Stderr, "warning: GOPATH set to GOROOT (%s) has no effect\n", gopath)
	}

	// Here, we need to go through and delete any reservations which should be expired.
	cleanOld()

	for _, cmd := range commands {
		if cmd.Name() == args[0] && cmd.Run != nil {
			cmd.Flag.Usage = func() { cmd.Usage() }
			if cmd.CustomFlags {
				args = args[1:]
			} else {
				cmd.Flag.Parse(args[1:])
				args = cmd.Flag.Args()
			}
			cmd.Run(cmd, args)
			exit()
			return
		}
	}

	fmt.Fprintf(os.Stderr, "go: unknown subcommand %q\nRun 'go help' for usage.\n", args[0])
	setExitStatus(2)
	exit()
}

var usageTemplate = `igor is a scheduler for Mega-style clusters.

Usage:

	igor command [arguments]

The commands are:
{{range .}}{{if .Runnable}}
    {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "igor help [command]" for more information about a command.

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

func exit() {
	os.Exit(exitStatus)
}

func fatalf(format string, args ...interface{}) {
	errorf(format, args...)
	exit()
}

func errorf(format string, args ...interface{}) {
	log.Printf(format, args...)
	setExitStatus(1)
}

type Reservation struct {
	ResName    string
	Hosts      []string // separate, not a range
	PXENames   []string // eg C000025B
	Expiration int64    // UNIX time
	Owner      string
}

func getReservations(f io.Reader) []Reservation {
	var ret []Reservation

	dec := json.NewDecoder(f)
	err := dec.Decode(&ret)
	// an empty file is OK, but other errors are not
	if err != nil && err != io.EOF {
		fatalf("failure parsing reservation file: %v", err)
	}

	return ret
}

// Convert an IP to a PXELinux-compatible string, i.e. 192.0.2.91 -> C000025B
func toPXE(ip net.IP) string {
	s := fmt.Sprintf("%02X%02X%02X%02X", ip[12], ip[13], ip[14], ip[15])
	return s
}
