package main

import (
	"flag"
	"fmt"
	log "minilog"
	"os"
)

var (
	f_serve    = flag.Bool("serve", false, "act as a server for enabled services")
	f_http     = flag.Bool("http", false, "enable http service")
	f_ssh      = flag.Bool("ssh", false, "enable ssh service")
	f_smtp     = flag.Bool("smtp", false, "enable smtp service")
	f_mean     = flag.Int("u", 100, "mean time, in milliseconds, between actions")
	f_variance = flag.Int("s", 0, "standard deviation between actions")
	f_loglevel = flag.String("loglevel", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("log", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
	hosts      map[string]string
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [flags] target(s)\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
  targets may include a hostname, ip address, subnet (for example 10.0.0.0/24) or
  comma seperated list of hosts/ip addresses (for example
  10.0.0.1,10.0.0.5,google.com). If a subnet or list is specified, protonuke will
  attempt to connect to each host simultaneously, so use with caution.
`)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	logSetup()

	hosts, err := parseHosts(flag.Args())
	if err != nil {
		log.Fatalln(err)
	}
	log.Infoln("hosts: ", hosts)
}
