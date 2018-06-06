// UFS is a userspace server which exports a filesystem over 9p2000.
//
// By default, it will export / over a TCP on port 5640 under the username
// of "harvey".
package main

import (
	"flag"
	"log"
	"net"

	"github.com/Harvey-OS/ninep/filesystem"
)

var (
	ntype = flag.String("ntype", "tcp4", "Default network type")
	naddr = flag.String("addr", ":5640", "Network address")
	root  = flag.String("root", "/", "Set the root for all attaches")
	debug = flag.Bool("debug", false, "print debug messages")
)

func main() {
	flag.Parse()
	l, err := net.Listen(*ntype, *naddr)
	if err != nil {
		log.Fatalf("Listen failed: %v", err)
	}

	fs := &ufs.FileServer{
		RootPath: *root,
		Debug:    *debug,
		Trace:    log.Printf,
	}

	if err := fs.Serve(l); err != nil {
		log.Fatal(err)
	}
}
