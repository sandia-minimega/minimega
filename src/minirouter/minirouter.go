// minirouter is a simple wrapper daemon for bird, dnsmasq, and iptool. It listens on a
// unix domain socket for updates and in turn updates configs for downstream
// services. minirouter also gathers statistics to populate vm tags on the host
// minimega instance and for use by the 'router' API in minimega.
package main

import (
	"flag"
	log "minilog"
	"os"
	"os/signal"
	"syscall"
)

var (
	f_loglevel = flag.String("level", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
	f_miniccc  = flag.String("miniccc", "/miniccc", "path to miniccc for sending logging and stats to minimega")
	f_path     = flag.String("path", "/tmp/minirouter", "base directory for minirouter")
)

func main() {
	// flags
	flag.Parse()

	logSetup()

	// attempt to set up the base path
	err := os.MkdirAll(*f_path, os.FileMode(0770))
	if err != nil {
		log.Fatal("mkdir base path: %v", err)
	}

	// start the domain socket service
	go commandSocketStart()

	// signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	<-sig

	// cleanup
}
