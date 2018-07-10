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
	Reset      = "\x1b[0m"
	Bright     = "\x1b[1m"
	Dim        = "\x1b[2m"
	Underscore = "\x1b[4m"
	Blink      = "\x1b[5m"
	Reverse    = "\x1b[7m"
	Hidden     = "\x1b[8m"

	FgBlack   = "\x1b[30m"
	FgRed     = "\x1b[31m"
	FgGreen   = "\x1b[32m"
	FgYellow  = "\x1b[33m"
	FgBlue    = "\x1b[34m"
	FgMagenta = "\x1b[35m"
	FgCyan    = "\x1b[36m"
	FgWhite   = "\x1b[37m"

	FgLightWhite = "\x1b[97m"

	BgBlack         = "\x1b[40m"
	BgRed           = "\x1b[41m"
	BgGreen         = "\x1b[42m"
	BgYellow        = "\x1b[43m"
	BgBlue          = "\x1b[44m"
	BgMagenta       = "\x1b[45m"
	BgCyan          = "\x1b[46m"
	BgWhite         = "\x1b[47m"
	BgBrightBlack   = "\x1b[100m"
	BgBrightRed     = "\x1b[101m"
	BgBrightGreen   = "\x1b[102m"
	BgBrightYellow  = "\x1b[103m"
	BgBrightBlue    = "\x1b[104m"
	BgBrightMagenta = "\x1b[105m"
	BgBrightCyan    = "\x1b[106m"
	BgBrightWhite   = "\x1b[107m"
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

	// Gather a list of which nodes are down
	var downNodes []string
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
		if maxResNameLength < len(r.ResName) {
			maxResNameLength = len(r.ResName)
		}
	}
	sort.Sort(StartSorter(resarray))

	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)

	printShelves(nodes, resarray)

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 10, 8, 0, '\t', 0)
	nameFmt := "%" + strconv.Itoa(maxResNameLength) + "v"
	//	fmt.Fprintf(w, "Reservations for cluster nodes %s[%d-%d]\n", igorConfig.Prefix, igorConfig.Start, igorConfig.End)
	fmt.Fprintln(w, fmt.Sprintf(nameFmt, "NAME"), "\t", "OWNER", "\t", "START", "\t", "END", "\t", "NODES")
	fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	downrange, _ := rnge.UnsplitRange(downNodes)
	name := BgRed + fmt.Sprintf(nameFmt, "DOWN") + Reset
	fmt.Fprintln(w, name, "\t", "N/A", "\t", "N/A", "\t", "N/A", "\t", downrange)
	w.Flush()
	timefmt := "Jan 2 15:04"
	for i, r := range resarray {
		resName := r.ResName
		unsplit, _ := rnge.UnsplitRange(r.Hosts)
		name = colorize(i, fmt.Sprintf(nameFmt, resName))
		fmt.Fprintln(w, name, "\t", r.Owner, "\t", time.Unix(r.StartTime, 0).Format(timefmt), "\t", time.Unix(r.EndTime, 0).Format(timefmt), "\t", unsplit)
		w.Flush()
	}
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
