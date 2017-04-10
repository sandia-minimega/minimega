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
	"math/rand"
	log "minilog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"
)

const MINUTES_PER_SLICE = 1 // must be less than 60!
const MIN_SCHED_LEN = 72    // minutes, 720 = 12 hours

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

// Represents a slice of time
type TimeSlice struct {
	Start int64    // UNIX time
	End   int64    // UNIX time
	Nodes []uint64 // slice of len(# of nodes), mapping to reservation IDs
}

type Reservation struct {
	ResName   string
	Hosts     []string // separate, not a range
	PXENames  []string // eg C000025B
	StartTime int64    // UNIX time
	EndTime   int64    // UNIX time
	Duration  float64  // minutes
	Owner     string
	ID        uint64
}

var Reservations map[uint64]Reservation // map ID to reservations
var Schedule []TimeSlice                // The schedule

var resdb *os.File
var scheddb *os.File

// Commands lists the available commands and help topics.
// The order here is the order in which they are printed by 'go help'.
var commands = []*Command{
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
		log.Fatal("Couldn't read config file: %v", err)
	}

	err = json.Unmarshal(b, &c)
	if err != nil {
		log.Fatal("Couldn't parse json: %v", err)
	}
	return
}

// Read the reservations, delete any that are too old.
// Copy in netboot files for any reservations that have just started
func tidyUp() {
	now := time.Now().Unix()

	for _, r := range Reservations {
		if r.EndTime < now {
			deleteReservation(false, []string{r.ResName})
		} else if r.StartTime < now {
			// Check if $TFTPROOT/pxelinux.cfg/igor/ResName exists
			filename := filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", "igor", r.ResName)
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				log.Info("Installing files for reservation ", r.ResName)

				// create appropriate pxe config file in igorConfig.TFTPRoot+/pxelinux.cfg/igor/
				fname := filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", "igor", r.ResName)
				masterfile, err := os.Create(fname)
				if err != nil {
					log.Fatal("failed to create %v -- %v", fname, err)
				}
				defer masterfile.Close()
				masterfile.WriteString(fmt.Sprintf("default %s\n\n", subR))
				masterfile.WriteString(fmt.Sprintf("label %s\n", subR))
				masterfile.WriteString(fmt.Sprintf("kernel /igor/%s-kernel\n", subR))
				masterfile.WriteString(fmt.Sprintf("append initrd=/igor/%s-initrd %s\n", subR, subC))

				// create individual PXE boot configs i.e. igorConfig.TFTPRoot+/pxelinux.cfg/AC10001B by copying config created above
				for _, pxename := range r.PXENames {
					masterfile.Seek(0, 0)
					fname := filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", pxename)
					f, err := os.Create(fname)
					if err != nil {
						log.Fatal("failed to create %v -- %v", fname, err)
					}
					io.Copy(f, masterfile)
					f.Close()
				}
			}
		}
	}

	ExpireSchedule()
	putSchedule()
}

func init() {
	Reservations = make(map[uint64]Reservation)
}

func main() {
	log.Init()

	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
	}

	if args[0] == "help" {
		help(args[1:])
		return
	}

	rand.Seed(time.Now().Unix())

	igorConfig = readConfig(*configpath)

	// Read in the reservations
	// We open the file here so resdb.Close() doesn't happen until program exit
	var err error
	path := filepath.Join(igorConfig.TFTPRoot, "/igor/reservations.json")
	resdb, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Fatal("failed to open reservations file: %v", err)
	}
	defer resdb.Close()
	err = syscall.Flock(int(resdb.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(resdb.Fd()), syscall.LOCK_UN) // this will unlock it later

	getReservations()

	// Read in the schedule
	path = filepath.Join(igorConfig.TFTPRoot, "/igor/schedule.json")
	scheddb, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Warn("failed to open schedule file: %v", err)
	}
	defer resdb.Close()
	err = syscall.Flock(int(resdb.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(resdb.Fd()), syscall.LOCK_UN) // this will unlock it later
	getSchedule()

	// Here, we need to go through and delete any reservations which should be expired,
	// and bring in new ones that are just starting
	tidyUp()

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
			return
		}
	}

	fmt.Fprintf(os.Stderr, "go: unknown subcommand %q\nRun 'go help' for usage.\n", args[0])
	setExitStatus(2)
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

func getReservations() {
	dec := json.NewDecoder(resdb)
	err := dec.Decode(&Reservations)
	// an empty file is OK, but other errors are not
	if err != nil && err != io.EOF {
		log.Fatal("failure parsing reservation file: %v", err)
	}
}

// Convert an IP to a PXELinux-compatible string, i.e. 192.0.2.91 -> C000025B
func toPXE(ip net.IP) string {
	s := fmt.Sprintf("%02X%02X%02X%02X", ip[12], ip[13], ip[14], ip[15])
	return s
}

func getSchedule() {
	dec := json.NewDecoder(scheddb)
	err := dec.Decode(&Schedule)
	// an empty file is OK, but other errors are not
	if err != nil && err != io.EOF {
		log.Fatal("failure parsing schedule file: %v", err)
	}
}

func putReservations() {
	// Truncate the existing reservation file
	resdb.Truncate(0)
	resdb.Seek(0, 0)
	// Write out the new reservations
	enc := json.NewEncoder(resdb)
	enc.Encode(Reservations)
	resdb.Sync()
}

func putSchedule() {
	// Truncate the existing schedule file
	scheddb.Truncate(0)
	scheddb.Seek(0, 0)
	// Write out the new schedule
	enc := json.NewEncoder(scheddb)
	enc.Encode(Schedule)
	scheddb.Sync()
}
