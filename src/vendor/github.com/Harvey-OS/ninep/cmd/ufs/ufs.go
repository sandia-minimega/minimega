// UFS is a userspace server which exports a filesystem over 9p2000.
//
// By default, it will export / over a TCP on port 5640 under the username
// of "harvey".
package main

import (
	"flag"
	"log"
	"net"

	"github.com/Harvey-OS/ninep/debug"
	"github.com/Harvey-OS/ninep/filesystem"
	"github.com/Harvey-OS/ninep/protocol"
)

var (
	ntype = flag.String("ntype", "tcp4", "Default network type")
	naddr = flag.String("addr", ":5640", "Network address")
	root  = flag.String("root", "/", "filesystem root")
	trace = flag.Bool("trace", false, "enable debug messages")
)

func checkErr(format string, err error) {
	if err != nil {
		log.Fatalf(format, err)
	}
}

func main() {
	flag.Parse()

	tracer := func(format string, args ...interface{}) {}
	if *trace {
		tracer = log.Printf
	}

	ln, err := net.Listen(*ntype, *naddr)
	checkErr("Listen failed: %v", err)

	fs, err := ufs.NewServer(ufs.Root(*root), ufs.Trace(tracer))
	checkErr("ufs.NewServer failed: %v", err)

	var ninefs protocol.NineServer = fs
	if *trace {
		ninefs, err = debug.NewServer(ninefs, debug.Trace(tracer))
		checkErr("debug.NewServer failed: %v", err)
	}

	s, err := protocol.NewServer(ninefs, protocol.Trace(tracer))
	checkErr("protocol.NewServer failed: %v", err)

	checkErr("Serve failed: %v", s.Serve(ln))
}
