// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	log "minilog"
	"os"
	"os/exec"
	"ranges"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
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

var cmdShow = &Command{
	UsageLine: "show",
	Short:     "show reservations",
	Long: `
List all extant reservations. Checks if a host is up by issuing a "ping"
	`,
}

func init() {
	// break init cycle
	cmdShow.Run = runShow
}

// Use nmap to scan all the nodes and then show which are up and the
// reservations they below to
func runShow(_ *Command, _ []string) {
	names := []string{}
	fmtstring := "%s%0" + strconv.Itoa(igorConfig.Padlen) + "d"
	for i := igorConfig.Start; i <= igorConfig.End; i++ {
		names = append(names, fmt.Sprintf(fmtstring, igorConfig.Prefix, i))
	}

	// Maps a node's index to a boolean value (up = true, down = false)
	nodes := map[int]bool{}
	// Maps a node's index to a boolean value (reserved = true, unreserved = false)
	resNodes := map[int]bool{}

	// Use nmap to determine what nodes are up
	args := []string{}
	if igorConfig.DNSServer != "" {
		args = append(args, "--dns-servers", igorConfig.DNSServer)
	}
	args = append(args,
		"-sn",
		"-PS22",
		"--unprivileged",
		"-T5",
		"-oG",
		"-",
	)
	cmd := exec.Command("nmap", append(args, names...)...)
	out, err := cmd.Output()
	if err != nil {
		log.Fatal("unable to scan: %v", err)
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
		v, err := strconv.Atoi(name[len(igorConfig.Prefix):])
		if err != nil {
			// that's weird
			continue
		}

		// If we found a node name in the output, that means it's up, so mark it as up
		nodes[v] = true
	}

	// Gather a list of which nodes are down and which nodes are unreserved
	var downNodes []string
	var unreservedNodes []string
	for i := igorConfig.Start; i <= igorConfig.End; i++ {
		if !nodes[i] {
			hostname := igorConfig.Prefix + strconv.Itoa(i)
			downNodes = append(downNodes, hostname)
		}
	}

	// For colors... get all the reservations and sort them
	resarray := []Reservation{}
	maxResNameLength := 0
	for _, r := range Reservations {
		resarray = append(resarray, r)
		// Remember longest reservation name for formatting
		if maxResNameLength < len(r.ResName) {
			maxResNameLength = len(r.ResName)
		}
		// go through each host list and compile list of reserved nodes
		for _, h := range r.Hosts {
			v, err := strconv.Atoi(h[len(igorConfig.Prefix):])
			if err != nil {
				//that's weird
				continue
			}
			resNodes[v] = true
		}
	}

	//compile a list of unreserved nodes
	for i := igorConfig.Start; i <= igorConfig.End; i++ {
		if !resNodes[i] {
			hostname := igorConfig.Prefix + strconv.Itoa(i)
			unreservedNodes = append(unreservedNodes, hostname)
		}
	}
	// nameFmt will create uniform color bars for 1st column
	nameFmt := "%" + strconv.Itoa(maxResNameLength) + "v"
	sort.Sort(StartSorter(resarray))

	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)

	printShelves(nodes, resarray)

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 0, 1, ' ', 0)
	// Header Row
	name := BgBlack + FgWhite + fmt.Sprintf(nameFmt, "NAME") + Reset
	fmt.Fprintln(w, name, "\t", "OWNER", "\t", "START", "\t", "END", "\t", "NODES")
	// Divider lines
	namedash := ""
	for i := 0; i < maxResNameLength; i++ {
		namedash += "-"
	}
	name = BgBlack + FgWhite + fmt.Sprintf(nameFmt, namedash) + Reset
	fmt.Fprintln(w, name, "\t", "-------", "\t", "------------", "\t", "------------", "\t", "------------")
	// "Down" Node list
	downrange, _ := rnge.UnsplitRange(downNodes)
	name = BgRed + FgWhite + fmt.Sprintf(nameFmt, "DOWN") + Reset
	fmt.Fprintln(w, name, "\t", "N/A", "\t", "N/A", "\t", "N/A", "\t", downrange)
	// Unreserved Node list
	resrange, _ := rnge.UnsplitRange(unreservedNodes)
	name = BgGreen + FgBlack + fmt.Sprintf(nameFmt, "UNRESERVED") + Reset
	fmt.Fprintln(w, name, "\t", "N/A", "\t", "N/A", "\t", "N/A", "\t", resrange)
	// Active Reservations
	timefmt := "Jan 2 15:04"
	for i, r := range resarray {
		unsplit, _ := rnge.UnsplitRange(r.Hosts)
		name = colorize(i, fmt.Sprintf(nameFmt, r.ResName))
		fmt.Fprintln(w, name, "\t", r.Owner, "\t", time.Unix(r.StartTime, 0).Format(timefmt), "\t", time.Unix(r.EndTime, 0).Format(timefmt), "\t", unsplit)
	}
	// only 1 flush at the end to ensure alignment
	w.Flush()
}

func printShelves(alive map[int]bool, resarray []Reservation) {
	// figure out how many digits we need per node displayed
	nodewidth := len(strconv.Itoa(igorConfig.End))
	nodefmt := "%" + strconv.Itoa(nodewidth) // for example, %3, for use as %3d or %3s

	// how many nodes per rack?
	perrack := igorConfig.Rackwidth * igorConfig.Rackheight

	// How wide is the full rack display?
	// width of nodes * number of nodes across a rack, plus the number of | characters we need
	totalwidth := nodewidth*igorConfig.Rackwidth + igorConfig.Rackwidth + 1

	// figure out all the node -> reservations ahead of time
	n2r := map[int]int{}
	now := time.Now().Unix()
	for i, r := range resarray {
		if r.StartTime < now {
			for _, name := range r.Hosts {
				name := strings.TrimPrefix(name, igorConfig.Prefix)
				v, err := strconv.Atoi(name)
				if err == nil {
					n2r[v] = i
				}
			}
		}
	}

	var buf bytes.Buffer
	for i := igorConfig.Start; i <= igorConfig.End; i += perrack {
		for j := 0; j < totalwidth; j++ {
			buf.WriteString(Reverse)
			buf.WriteString("-")
			buf.WriteString(Reset)
		}
		buf.WriteString("\n")
		for j := i; j < i+perrack; j++ {
			if (j-1)%igorConfig.Rackwidth == 0 {
				buf.WriteString(Reverse)
				buf.WriteString("|")
				buf.WriteString(Reset)
			}
			if j <= igorConfig.End {
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
			if (j-1)%igorConfig.Rackwidth == igorConfig.Rackwidth-1 {
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
