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
	path := TFTPROOT + "/igor/reservations.json"
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

	rnge, _ := ranges.NewRange(PREFIX, START, END)

	fmt.Printf("RESERVATION NAME      OWNER      TIME REMAINING      NODES\n")
	fmt.Printf("--------------------------------------------------------------------------------\n")
	for _, r := range reservations {
		unsplit, _ := rnge.UnsplitRange(r.Hosts)
		fmt.Printf("%-22s%-11s%-20.1f%s\n",  r.ResName, r.Owner, time.Now().Sub(time.Unix(r.Expiration, 0)).Hours(), unsplit)
	}
	//fmt.Println(reservations)
}
