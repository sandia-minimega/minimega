package main

import (
	"flag"
	"fmt"
	log "minilog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	f_serve    = flag.Bool("serve", false, "act as a server for enabled services")
	f_http     = flag.Bool("http", false, "enable http service")
	f_https    = flag.Bool("https", false, "enable https (TLS) service")
	f_ssh      = flag.Bool("ssh", false, "enable ssh service")
	f_smtp     = flag.Bool("smtp", false, "enable smtp service")
	f_mean     = flag.Int("u", 100, "mean time, in milliseconds, between actions")
	f_stddev   = flag.Int("s", 0, "standard deviation between actions")
	f_min      = flag.Int("min", 0, "minimum time allowable for events, in milliseconds")
	f_max      = flag.Int("max", 60000, "maximum time allowable for events, in milliseconds")
	f_loglevel = flag.String("loglevel", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("log", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
	f_report   = flag.Duration("report", time.Duration(10*time.Second), "time between reports, set to 0 to disable")
	hosts      map[string]string
	keys       []string
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

	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, syscall.SIGINT)

	logSetup()

	// make sure at least one service is enabled
	if !*f_http && !*f_https && !*f_ssh && !*f_smtp {
		log.Fatalln("no enabled services")
	}

	// make sure mean and variance are > 0
	if *f_mean < 0 || *f_stddev < 0 || *f_min < 0 || *f_max < 0 {
		log.Fatalln("mean, standard deviation, min, and max must be > 0")
	}

	var err error
	hosts, keys, err = parseHosts(flag.Args())
	if err != nil {
		log.Fatalln(err)
	}
	if len(hosts) == 0 && !*f_serve {
		log.Fatalln("no hosts specified")
	}
	log.Infoln("hosts: ", hosts)

	// start the reporter
	if *f_report == 0 {
		log.Debugln("disabling reports")
	} else {
		log.Debug("enabling reports every %v", *f_report)
		go report(*f_report)
	}

	// start services
	if *f_http {
		if *f_serve {
			go httpServer()
		} else {
			go httpClient()
		}
	}
	if *f_https {
		if *f_serve {
			go httpTLSServer()
		} else {
			go httpTLSClient()
		}
	}
	<-sig
}
