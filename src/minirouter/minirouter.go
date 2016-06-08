// minirouter is a simple wrapper daemon for bird, dnsmasq, and iptool. It listens on a
// unix domain socket for updates and in turn updates configs for downstream
// services. minirouter also gathers statistics to populate vm tags on the host
// minimega instance and for use by the 'router' API in minimega.
package main

import (
	"flag"
)

var (
	f_loglevel   = flag.String("level", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log        = flag.Bool("v", true, "log on stderr")
	f_logfile    = flag.String("logfile", "", "also log to file")
)

func main () {
	// flags
	flag.Parse()

	logSetup()

	// start the domain socket service

	// signal handling
}
