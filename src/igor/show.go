// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	log "minilog"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// Some color constants for output
const (
	Reset      = "\x1b[0000m"
	Bright     = "\x1b[0001m"
	Dim        = "\x1b[0002m"
	Underscore = "\x1b[0004m"
	Blink      = "\x1b[0005m"
	Reverse    = "\x1b[0007m"
	Hidden     = "\x1b[0008m"

	FgBlack   = "\x1b[0030m"
	FgRed     = "\x1b[0031m"
	FgGreen   = "\x1b[0032m"
	FgYellow  = "\x1b[0033m"
	FgBlue    = "\x1b[0034m"
	FgMagenta = "\x1b[0035m"
	FgCyan    = "\x1b[0036m"
	FgWhite   = "\x1b[0037m"

	FgLightWhite = "\x1b[0097m"

	BgBlack         = "\x1b[0040m"
	BgRed           = "\x1b[0041m"
	BgGreen         = "\x1b[0042m"
	BgYellow        = "\x1b[0043m"
	BgBlue          = "\x1b[0044m"
	BgMagenta       = "\x1b[0045m"
	BgCyan          = "\x1b[0046m"
	BgWhite         = "\x1b[0047m"
	BgBrightBlack   = "\x1b[0100m"
	BgBrightRed     = "\x1b[0101m"
	BgBrightGreen   = "\x1b[0102m"
	BgBrightYellow  = "\x1b[0103m"
	BgBrightBlue    = "\x1b[0104m"
	BgBrightMagenta = "\x1b[0105m"
	BgBrightCyan    = "\x1b[0106m"
	BgBrightWhite   = "\x1b[0107m"
)

const showTimeFmt = "Jan 2 15:04"

type rowPrinter struct {
	w       io.Writer
	nameFmt string

	showColors bool
}

var cmdShow = &Command{
	UsageLine: "show [OPTION]...",
	Short:     "show reservations",
	Long: `
Show node status and list reservations. Checks if a node is up by issuing a
"ping". By default, reservations are sorted by start time. Each reservation
has a set of associated flags:

	A: Reservation is active
	W: Reservation is writable by current user
	I: Reservation is installed
	E: Reservation had an error during install

All active reservations will be either installed or in an error state. Use
"igor show -e" to show install errors.

If a reservation is writable, that means that that the current user can perform
power operations, edit, delete, or extend the reservation. Writable is
determined based on the reservation's owner and group.

OPTIONAL FLAGS:

Sorting:

	-o: change the sort order to sort by reservation owner
	-n: change the sort order to sort by reservation name
	-r: reverse the order while sorting

Filtering (may be used together):

	-owner: filter reservations based on owner
	-group: filter reservations based on group
	-name: filter reservations based on name
	-active: only show active reservations
	-future: only show future reservations
	-installed: only show installed reservations
	-errored: only show install errored reservations
	-writable: only show writable reservations

Formatting:

	-c: shows colors (default true, use -c=false to disable colors)
	-t: show node status table (default true, use -t=false to disable)
	-e: prints install errors for reservations (ignores other flags)
	`,
}

var showOpts struct {
	sortOwner bool
	sortName  bool
	reverse   bool

	filterOwner     string
	filterName      string
	filterGroup     string
	filterActive    bool
	filterFuture    bool
	filterInstalled bool
	filterErrored   bool
	filterWritable  bool

	showColors bool
	showTable  bool
	showErrors bool
}

func init() {
	// break init cycle
	cmdShow.Run = runShow

	cmdShow.Flag.BoolVar(&showOpts.sortOwner, "o", false, "sort by owner")
	cmdShow.Flag.BoolVar(&showOpts.sortName, "n", false, "sort by reservation name")
	cmdShow.Flag.BoolVar(&showOpts.reverse, "r", false, "reverse order while sorting")

	cmdShow.Flag.StringVar(&showOpts.filterOwner, "owner", "", "filter by owner")
	cmdShow.Flag.StringVar(&showOpts.filterGroup, "group", "", "filter by group")
	cmdShow.Flag.StringVar(&showOpts.filterName, "name", "", "filter by name")
	cmdShow.Flag.BoolVar(&showOpts.filterActive, "active", false, "only show active reservations")
	cmdShow.Flag.BoolVar(&showOpts.filterFuture, "future", false, "only show future reservations")
	cmdShow.Flag.BoolVar(&showOpts.filterInstalled, "installed", false, "only show installed reservations")
	cmdShow.Flag.BoolVar(&showOpts.filterErrored, "errored", false, "only show install errored reservations")
	cmdShow.Flag.BoolVar(&showOpts.filterWritable, "writable", false, "only show writable reservations")

	cmdShow.Flag.BoolVar(&showOpts.showColors, "c", true, "show colors")
	cmdShow.Flag.BoolVar(&showOpts.showTable, "t", true, "show node status table")
	cmdShow.Flag.BoolVar(&showOpts.showErrors, "e", false, "show install errors")
}

// Use nmap to scan all the nodes and then show which are up and the
// reservations they below to
func runShow(_ *Command, _ []string) {
	// Show reservations with errors and return
	if showOpts.showErrors {
		// check to see that there are install errors first
		var count int

		// TODO: probably shouldn't iteration over .M directly
		for _, r := range igor.Reservations.M {
			if r.InstallError != "" {
				count += 1
			}
		}
		if count == 0 {
			return
		}

		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 0, 0, 1, ' ', 0)
		fmt.Fprintln(w, "Reservation", "\t", "Error")

		// TODO: probably shouldn't iteration over .M directly
		for _, r := range igor.Reservations.M {
			if r.InstallError != "" {
				fmt.Fprintln(w, r.Name, "\t", r.InstallError)
			}
		}

		w.Flush()
		return
	}

	names := igor.validHosts()

	// Maps a node's index to a boolean value (up = true, down = false)
	nodes := map[int]bool{}
	if showOpts.showTable {
		n, err := scanNodes(names)
		if err != nil {
			log.Fatal("unable to scan: %v", err)
		}
		nodes = n
	}

	// Maps a node's index to a boolean value (reserved = true, unreserved = false)
	resNodes := map[int]bool{}

	// For colors... get all the reservations and sort them
	resarray := []*Reservation{}
	maxResNameLength := 0

	// TODO: probably shouldn't iteration over .M directly
	for _, r := range igor.Reservations.M {
		resarray = append(resarray, r)
		// Remember longest reservation name for formatting
		if maxResNameLength < len(r.Name) {
			maxResNameLength = len(r.Name)
		}
		// go through each host list and compile list of reserved nodes
		for _, h := range r.Hosts {
			v, err := strconv.Atoi(h[len(igor.Prefix):])
			if err != nil {
				//that's weird
				continue
			}
			resNodes[v] = true
		}
	}

	// Gather a list of which nodes are down and which nodes are unreserved
	var downNodes []string
	var unreservedNodes []string
	for i := igor.Start; i <= igor.End; i++ {
		hostname := igor.Prefix + strconv.Itoa(i)
		if !resNodes[i] {
			unreservedNodes = append(unreservedNodes, hostname)
		}
		if !nodes[i] {
			downNodes = append(downNodes, hostname)
		}
	}

	if showOpts.showTable {
		printShelves(nodes, resarray)
	}

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 0, 1, ' ', 0)

	p := rowPrinter{
		// nameFmt will create uniform color bars for 1st column
		nameFmt:    "%" + strconv.Itoa(maxResNameLength) + "v",
		showColors: showOpts.showColors,
		w:          w,
	}

	// Header Row
	p.printHeader()
	p.printSpacer()

	// "Down" Node list, only available if we scanned to create the table
	if showOpts.showTable {
		p.printHosts("DOWN", BgRed+FgWhite, downNodes)
	}

	// Unreserved Node list
	p.printHosts("UNRESERVED", BgGreen+FgBlack, unreservedNodes)
	p.printSpacer()

	// Filter and sort the reservations, if any
	resarray = filterReservations(resarray)
	sortReservations(resarray)

	p.printReservations(resarray)

	// only 1 flush at the end to ensure alignment
	w.Flush()
}

// scanNodes checks to see what hosts are up/down from the list. Returns a map
// where indices in nodes correspond to up/down.
func scanNodes(nodes []string) (map[int]bool, error) {
	res := map[int]bool{}

	// Use nmap to determine what nodes are up
	args := []string{}
	if igor.DNSServer != "" {
		args = append(args, "--dns-servers", igor.DNSServer)
	}
	args = append(args,
		"-sn",
		"-PS22",
		"--unprivileged",
		"-T5",
		"-oG",
		"-",
	)
	cmd := exec.Command("nmap", append(args, nodes...)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	s := bufio.NewScanner(bytes.NewReader(out))

	// Parse the results of nmap
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 5 {
			// that's weird
			continue
		}

		// trim off ()
		name := fields[2][1 : len(fields[2])-1]
		// get rid of any domain that may exist
		name = strings.Split(name, ".")[0]
		v, err := strconv.Atoi(name[len(igor.Prefix):])
		if err != nil {
			// that's weird
			continue
		}

		// If we found a node name in the output, that means it's up, so mark it as up
		res[v] = true
	}

	return res, nil
}

// filterReservations filters the reservations based on showOpts
func filterReservations(rs []*Reservation) []*Reservation {
	rs2 := []*Reservation{}

	for _, r := range rs {
		if !strings.Contains(r.Owner, showOpts.filterOwner) {
			continue
		}

		if !strings.Contains(r.Group, showOpts.filterGroup) {
			continue
		}

		if !strings.Contains(r.Name, showOpts.filterName) {
			continue
		}

		if showOpts.filterActive && !r.IsActive(igor.Now) {
			continue
		}

		if showOpts.filterFuture && r.IsActive(igor.Now) {
			continue
		}

		if showOpts.filterInstalled && !r.Installed {
			continue
		}

		if showOpts.filterErrored && r.InstallError != "" {
			continue
		}

		if showOpts.filterWritable && !r.IsWritable(igor.User) {
			continue
		}

		// passed all filters (or none set)
		rs2 = append(rs2, r)
	}

	return rs2
}

// sortReservations sorts the reservations based on showOpts
func sortReservations(rs []*Reservation) {
	sortStart := func(i, j int) bool {
		return rs[i].Start.Before(rs[j].Start)
	}
	sortFn := sortStart

	if showOpts.sortOwner {
		sortFn = func(i, j int) bool {
			if rs[i].Owner == rs[j].Owner {
				return sortStart(i, j)
			}
			return rs[i].Owner < rs[j].Owner
		}
	} else if showOpts.sortName {
		sortFn = func(i, j int) bool {
			if rs[i].Name == rs[j].Name {
				return sortStart(i, j)
			}
			return rs[i].Name < rs[j].Name
		}
	}

	sort.Slice(rs, sortFn)

	if showOpts.reverse {
		// From golang's SliceTricks
		for i := len(rs)/2 - 1; i >= 0; i-- {
			opp := len(rs) - 1 - i
			rs[i], rs[opp] = rs[opp], rs[i]
		}
	}
}

func printShelves(alive map[int]bool, resarray []*Reservation) {
	// figure out how many digits we need per node displayed
	nodewidth := len(strconv.Itoa(igor.End))
	nodefmt := "%" + strconv.Itoa(nodewidth) // for example, %3, for use as %3d or %3s

	// how many nodes per rack?
	perrack := igor.Rackwidth * igor.Rackheight

	// How wide is the full rack display?
	// width of nodes * number of nodes across a rack, plus the number of | characters we need
	totalwidth := nodewidth*igor.Rackwidth + igor.Rackwidth + 1

	// figure out all the node -> reservations ahead of time
	n2r := map[int]int{}
	for i, r := range resarray {
		if r.Start.Before(igor.Now) {
			for _, name := range r.Hosts {
				name := strings.TrimPrefix(name, igor.Prefix)
				v, err := strconv.Atoi(name)
				if err == nil {
					n2r[v] = i
				}
			}
		}
	}

	var buf bytes.Buffer
	for i := igor.Start; i <= igor.End; i += perrack {
		for j := 0; j < totalwidth; j++ {
			buf.WriteString(Reverse)
			buf.WriteString("-")
			buf.WriteString(Reset)
		}
		buf.WriteString("\n")
		for j := i; j < i+perrack; j++ {
			if (j-1)%igor.Rackwidth == 0 {
				buf.WriteString(Reverse)
				buf.WriteString("|")
				buf.WriteString(Reset)
			}
			if j <= igor.End {
				if index, ok := n2r[j]; ok {
					if alive[j] {
						buf.WriteString(colorize(index, fmt.Sprintf(nodefmt+"d", j)))
					} else {
						buf.WriteString(BgRed)
						fmt.Fprintf(&buf, nodefmt+"d", j)
						buf.WriteString(Reset)
					}
				} else {
					if alive[j] {
						fmt.Fprintf(&buf, nodefmt+"d", j)
					} else {
						buf.WriteString(BgRed)
						fmt.Fprintf(&buf, nodefmt+"d", j)
						buf.WriteString(Reset)
					}
				}
			} else {
				fmt.Fprintf(&buf, nodefmt+"s", " ")
			}
			buf.WriteString(Reverse)
			buf.WriteString("|")
			buf.WriteString(Reset)
			if (j-1)%igor.Rackwidth == igor.Rackwidth-1 {
				buf.WriteString("\n")
			}
		}
		for j := 0; j < totalwidth; j++ {
			buf.WriteString(Reverse)
			buf.WriteString("-")
			buf.WriteString(Reset)
		}
		buf.WriteString("\n\n")
	}
	fmt.Print(buf.String())
}

func (p rowPrinter) printHeader() {
	name := fmt.Sprintf(p.nameFmt, "NAME")
	if p.showColors {
		name = BgBlack + FgWhite + name + Reset
	}

	fmt.Fprintln(p.w,
		name, "\t",
		"OWNER", "\t",
		"START", "\t",
		"END", "\t",
		"FLAGS", "\t",
		"SIZE", "\t",
		"NODES")
}

func (p rowPrinter) printSpacer() {
	name := strings.Replace(fmt.Sprintf(p.nameFmt, ""), " ", "-", -1)
	if p.showColors {
		name = BgBlack + FgWhite + name + Reset
	}

	fmt.Fprintln(p.w,
		name, "\t",
		"-------", "\t",
		"------------", "\t",
		"------------", "\t",
		"------", "\t",
		"-----", "\t",
		"------------")
}

func (p rowPrinter) printHosts(name, color string, hosts []string) {
	name = fmt.Sprintf(p.nameFmt, name)
	if p.showColors {
		name = color + name + Reset
	}

	fmt.Fprintln(p.w,
		name, "\t",
		"N/A", "\t",
		"N/A", "\t",
		"N/A", "\t",
		"N/A", "\t",
		len(hosts), "\t",
		igor.unsplitRange(hosts))
}

func (p rowPrinter) printReservations(rs []*Reservation) {
	for i, r := range rs {
		name := fmt.Sprintf(p.nameFmt, r.Name)
		if p.showColors {
			name = colorize(i, name)
		}

		fmt.Fprintln(p.w,
			name, "\t",
			r.Owner, "\t",
			r.Start.Format(showTimeFmt), "\t",
			r.End.Format(showTimeFmt), "\t",
			r.Flags(igor.Now), "\t",
			len(r.Hosts), "\t",
			igor.unsplitRange(r.Hosts))
	}
}

func colorize(index int, str string) string {
	return fgColors[index%len(fgColors)] + bgColors[index%len(bgColors)] + str + Reset
}

var fgColors = []string{
	FgLightWhite,
	FgLightWhite,
	FgLightWhite,
	FgLightWhite,
	FgBlack,
	FgBlack,
	FgBlack,
	FgBlack,
	FgBlack,
	FgBlack,
}

var bgColors = []string{
	BgGreen,
	BgBlue,
	BgMagenta,
	BgCyan,
	BgYellow,
	BgBrightGreen,
	BgBrightBlue,
	BgBrightMagenta,
	BgBrightCyan,
	BgBrightYellow,
}
