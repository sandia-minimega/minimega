// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"ranges"
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

	names := []string{}
	for i := igorConfig.Start; i <= igorConfig.End; i++ {
		names = append(names, igorConfig.Prefix+strconv.Itoa(i))
	}

	nodes := map[int]bool{}

	args := []string{
		"-sn",
		"--max-retries=1",
		"--host-timeout=10ms",
		"-oG",
		"-",
	}

	cmd := exec.Command("nmap", append(args, names...)...)

	out, err := cmd.Output()
	if err != nil {
		fatalf("unable to scan: %v", err)
	}

	s := bufio.NewScanner(bytes.NewReader(out))

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

		v, err := strconv.Atoi(name[len(igorConfig.Prefix):])
		if err != nil {
			// that's weird
			continue
		}

		nodes[v] = true
	}

	var downNodes []string
	for i, alive := range nodes {
		if !alive {
			hostname := igorConfig.Prefix + strconv.Itoa(i)
			downNodes = append(downNodes, hostname)
		}
	}

	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)

	printShelves(reservations, nodes)

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 10, 8, 0, '\t', 0)

	//	fmt.Fprintf(w, "Reservations for cluster nodes %s[%d-%d]\n", igorConfig.Prefix, igorConfig.Start, igorConfig.End)
	fmt.Fprintln(w, "NAME", "\t", "OWNER", "\t", "TIME LEFT", "\t", "NODES")
	fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
	w.Flush()
	downrange, _ := rnge.UnsplitRange(downNodes)
	fmt.Print(BgRed + "DOWN" + Reset)
	fmt.Fprintln(w, "\t", "N/A", "\t", "N/A", "\t", downrange)
	w.Flush()
	for idx, r := range reservations {
		unsplit, _ := rnge.UnsplitRange(r.Hosts)
		timeleft := fmt.Sprintf("%.1f", time.Unix(r.Expiration, 0).Sub(time.Now()).Hours())
		//		fmt.Fprintln(w, colorize(idx, r.ResName), "\t", r.Owner, "\t", timeleft, "\t", unsplit)
		fmt.Print(colorize(idx, r.ResName))
		fmt.Fprintln(w, "\t", r.Owner, "\t", timeleft, "\t", unsplit)
		w.Flush()
	}
	w.Flush()
}

func printShelves(reservations []Reservation, alive map[int]bool) {
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
	for i, r := range reservations {
		for _, name := range r.Hosts {
			name := strings.TrimPrefix(name, igorConfig.Prefix)
			v, err := strconv.Atoi(name)
			if err == nil {
				n2r[v] = i
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

func resContains(reservations []Reservation, node string) (bool, int) {
	for idx, r := range reservations {
		for _, name := range r.Hosts {
			if name == node {
				return true, idx
			}
		}
	}
	return false, 0
}

func colorize(index int, str string) string {
	return colors[index%len(colors)] + str + Reset
}

var colors = []string{
	BgGreen,
	BgYellow,
	BgBlue,
	BgMagenta,
	BgCyan,
	BgBrightBlack,
	BgBrightGreen,
	BgBrightYellow,
	BgBrightBlue,
	BgBrightMagenta,
	BgBrightCyan,
}
