package main

import (
	"fmt"
	"os"
	"time"
	"ranges"
)

var cmdShow = &Command{
	UsageLine: "show",
	Short:	"show reservations",
	Long:`
List all extant reservations. Checks if a host is up by issuing a "ping"
	`,
}

func init() {
	// break init cycle
	cmdShow.Run = runShow
}

// Ping every node (concurrently), then show which nodes are up
// and which nodes are in which reservation
// TODO: implement real pinging
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

	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)

	printShelves(reservations)

	fmt.Printf("Reservations for cluster nodes %s[%d-%d]\n", igorConfig.Prefix, igorConfig.Start, igorConfig.End)
	fmt.Printf("RESERVATION NAME      OWNER      TIME REMAINING      NODES\n")
	fmt.Printf("--------------------------------------------------------------------------------\n")
	for idx, r := range reservations {
		unsplit, _ := rnge.UnsplitRange(r.Hosts)
		fmt.Printf("%-22s%-11s%-20.1f%s\n",  colorize(idx, r.ResName), r.Owner, time.Unix(r.Expiration, 0).Sub(time.Now()).Hours(), unsplit)
	}
	//fmt.Println(reservations)
}

func printShelves(reservations []Reservation) {
	// figure out how many digits we need per node displayed
	nodewidth := len(fmt.Sprintf("%d", igorConfig.End))

	// how many nodes per rack?
	perrack := igorConfig.Rackwidth * igorConfig.Rackheight

	// How wide is the full rack display?
	// width of nodes * number of nodes across a rack, plus the number of | characters we need
	totalwidth := nodewidth * igorConfig.Rackwidth + igorConfig.Rackwidth + 1

	output := ""
	for i := igorConfig.Start; i <= igorConfig.End; i += perrack {
		for j := 0; j < totalwidth; j++ {
			output += outline("-")
		}
		output += "\n"
		for j := i; j < i + perrack; j++ {
			if (j - 1) % igorConfig.Rackwidth == 0 {
				output += outline("|")
			}
			if contains, index := resContains(reservations, fmt.Sprintf("%s%d", igorConfig.Prefix, j)); contains {
				output += colorize(index, fmt.Sprintf("%3d", j))
			} else {
				output += fmt.Sprintf("%3d", j)
			}
			output += outline("|")
			if (j -1) % igorConfig.Rackwidth == igorConfig.Rackwidth - 1 {
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
    return "\033[7m" + str + "\033[0m"
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
	return colors[index % len(colors)] + str + "\033[0m"
}

var colors = []string{
//	"\033[40m",	// black
//	"\033[41m",	// red is reserved
	"\033[42m",
	"\033[43m",
	"\033[44m",
	"\033[45m",
	"\033[46m",
//	"\033[47m",	// light gray is too light on white terminals
	"\033[100m",	// dark gray
//	"\033[101m",	// light red is reserved
	"\033[102m",
	"\033[103m",
	"\033[104m",
	"\033[105m",
	"\033[106m",
}
