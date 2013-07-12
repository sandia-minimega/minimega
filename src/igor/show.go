// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"ranges"
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

// Ping a host, return true if it is alive
func isAlive(host string) bool {
	cmd := exec.Command("ping", "-c1", "-W1", host)
	err := cmd.Run()
	if err != nil {
		return false
	}
	return true
}

// Ping every node (concurrently), then show which nodes are up
// and which nodes are in which reservation
func runShow(cmd *Command, args []string) {
	path := igorConfig.TFTPRoot + "/igor/reservations.json"
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

	// Find out what nodes are down
	nodesAlive := make(map[int]bool) // if node is alive, set bool to true
	done := make(chan bool)
	fifo := make(chan int, 200)
	for i := igorConfig.Start; i <= igorConfig.End; i++ {
		fifo <- 1
		go func(i int) {
			hostname := fmt.Sprintf("%s%d", igorConfig.Prefix, i)
			nodesAlive[i] = isAlive(hostname)
			<-fifo
			done <- true
		}(i)
	}
	for i := igorConfig.Start; i <= igorConfig.End; i++ {
		<-done
	}
	var downNodes []string
	for number, isup := range nodesAlive {
		if !isup {
			hostname := fmt.Sprintf("%s%d", igorConfig.Prefix, number)
			downNodes = append(downNodes, hostname)
		}
	}

	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)

	printShelves(reservations, nodesAlive)

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
	nodewidth := len(fmt.Sprintf("%d", igorConfig.End))
	nodefmt := "%" + fmt.Sprintf("%d", nodewidth) // for example, %3, for use as %3d or %3s

	// how many nodes per rack?
	perrack := igorConfig.Rackwidth * igorConfig.Rackheight

	// How wide is the full rack display?
	// width of nodes * number of nodes across a rack, plus the number of | characters we need
	totalwidth := nodewidth*igorConfig.Rackwidth + igorConfig.Rackwidth + 1

	output := ""
	for i := igorConfig.Start; i <= igorConfig.End; i += perrack {
		for j := 0; j < totalwidth; j++ {
			output += outline("-")
		}
		output += "\n"
		for j := i; j < i+perrack; j++ {
			if (j-1)%igorConfig.Rackwidth == 0 {
				output += outline("|")
			}
			if j <= igorConfig.End {
				if contains, index := resContains(reservations, fmt.Sprintf("%s%d", igorConfig.Prefix, j)); contains {
					if alive[j] {
						output += colorize(index, fmt.Sprintf(nodefmt+"d", j))
					} else {
						output += BgRed + fmt.Sprintf(nodefmt+"d", j) + Reset
					}
				} else {
					if alive[j] {
						output += fmt.Sprintf(nodefmt+"d", j)
					} else {
						output += BgRed + fmt.Sprintf(nodefmt+"d", j) + Reset
					}
				}
			} else {
				output += fmt.Sprintf(nodefmt+"s", " ")
			}
			output += outline("|")
			if (j-1)%igorConfig.Rackwidth == igorConfig.Rackwidth-1 {
				output += "\n"
			}
		}
		for j := 0; j < totalwidth; j++ {
			output += outline("-")
		}
		output += "\n\n"
	}
	fmt.Print(output)
}

func outline(str string) string {
	return Reverse + str + Reset
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
