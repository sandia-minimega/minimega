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
	f_httproot = flag.String("httproot", "", "serve directory with http(s) instead of the builtin page generator")
	f_ssh      = flag.Bool("ssh", false, "enable ssh service")
	f_smtp     = flag.Bool("smtp", false, "enable smtp service")
	f_smtpUser = flag.String("smtpuser", "", "specify a particular user to send email to for the given domain, otherwise random")
	f_smtpTls  = flag.Bool("smtptls", true, "enable or disable sending mail with TLS")
	f_mean     = flag.Duration("u", time.Duration(1000*time.Millisecond), "mean time between actions")
	f_stddev   = flag.Duration("s", time.Duration(0), "standard deviation between actions")
	f_min      = flag.Duration("min", time.Duration(0), "minimum time allowable for events")
	f_max      = flag.Duration("max", time.Duration(60000*time.Millisecond), "maximum time allowable for events")
	f_loglevel = flag.String("loglevel", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("log", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
	f_v4       = flag.Bool("ipv4", true, "use IPv4. Can be used together with -ipv6")
	f_v6       = flag.Bool("ipv6", true, "use IPv6. Can be used together with -ipv4")
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

	var protocol string
	if *f_v4 && *f_v6 {
		protocol = "tcp"
	} else if *f_v4 && !*f_v6 {
		protocol = "tcp4"
	} else if !*f_v4 && *f_v6 {
		protocol = "tcp6"
	} else {
		log.Fatalln("you must enable at least one of IPv4 or IPv6")
	}

	// start services
	if *f_http {
		if *f_serve {
			go httpServer(protocol)
		} else {
			go httpClient()
		}
	}
	if *f_https {
		if *f_serve {
			go httpTLSServer(protocol)
		} else {
			go httpTLSClient()
		}
	}
	if *f_ssh {
		if *f_serve {
			go sshServer(protocol)
		} else {
			go sshClient()
		}
	}
	if *f_smtp {
		if *f_serve {
			go smtpServer(protocol)
		} else {
			go smtpClient()
		}
	}
	<-sig
}
