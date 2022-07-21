// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

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
	-json: prints reservation info as a JSON Object
	`,
}

var showOpts struct {
	sortOwner bool
	sortName  bool
	reverse   bool

	asJSON bool

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

	cmdShow.Flag.BoolVar(&showOpts.asJSON, "json", false, "print JSON-encoded reservation info")

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
	maxResNameLength := len("UNRESERVED") // always included

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

	// sort according to options
	sortReservations(resarray)

	// if printing as JSON, write out info and bail...
	if showOpts.asJSON {
		data, err := json.Marshal(struct {
			Prefix                                      string
			RangeStart, RangeEnd, RackWidth, RackHeight int
			Available, Down                             []string
			Reservations                                []*Reservation
		}{
			Prefix:       igor.Config.Prefix,
			RangeStart:   igor.Config.Start,
			RangeEnd:     igor.Config.End,
			RackWidth:    igor.Config.Rackwidth,
			RackHeight:   igor.Config.Rackheight,
			Available:    unreservedNodes,
			Down:         downNodes,
			Reservations: resarray,
		})
		if err != nil {
			log.Fatal("unable to marshal reservations: %v", err)
		}

		fmt.Printf("%s\n", data)
		return
	}

	// ... if not printing as json
	if showOpts.showTable {
		p := tablePrinter{
			filter:     isFiltered,
			showColors: showOpts.showColors,
			alive:      nodes,
		}
		p.printTable(resarray)
	}

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 0, 1, ' ', 0)

	p := rowPrinter{
		filter:     isFiltered,
		showColors: showOpts.showColors,

		nameFmt: "%" + strconv.Itoa(maxResNameLength) + "v",
		timeFmt: "Jan 2 15:04",

		w: w,
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

	// Finally, print all the reservations
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
		// scan all the nodes in parallel since we really shouldn't have that
		// many hosts to scan
		"--min-parallelism",
		strconv.Itoa(len(nodes)),
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

// isFiltered tests whether a reservation should be filtered or not based on
// showOpts
func isFiltered(r *Reservation) bool {
	if !strings.Contains(r.Owner, showOpts.filterOwner) {
		return true
	}

	if !strings.Contains(r.Group, showOpts.filterGroup) {
		return true
	}

	if !strings.Contains(r.Name, showOpts.filterName) {
		return true
	}

	if showOpts.filterActive && !r.IsActive(igor.Now) {
		return true
	}

	if showOpts.filterFuture && r.IsActive(igor.Now) {
		return true
	}

	if showOpts.filterInstalled && !r.Installed {
		return true
	}

	if showOpts.filterErrored && r.InstallError != "" {
		return true
	}

	if showOpts.filterWritable && !r.IsWritable(igor.User) {
		return true
	}

	return false
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
