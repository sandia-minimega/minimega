// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

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
	f_serve         = flag.Bool("serve", false, "act as a server for enabled services")
	f_dns           = flag.Bool("dns", false, "enable dns service")
	f_dnsv4         = flag.Bool("dnsv4", false, "dns client only requests type A records")
	f_dnsv6         = flag.Bool("dnsv6", false, "dns client only requests type AAAA records")
	f_randomhosts   = flag.Bool("random-hosts", false, "if no host range is supplied return a randomly generated ip")
	f_http          = flag.Bool("http", false, "enable http service")
	f_https         = flag.Bool("https", false, "enable https (TLS) service")
	f_httproot      = flag.String("httproot", "", "serve directory with http(s) instead of the builtin page generator")
	f_httpGzip      = flag.Bool("httpgzip", false, "gzip image served in http/https pages")
	f_irc           = flag.Bool("irc", false, "enable irc service")
	f_ircport       = flag.String("ircport", "6667", "port to use for IRC client or server")
	f_channels      = flag.String("channels", "#general,#random", "overwrite default IRC channels to join, seperated by commas")
	f_messages      = flag.String("messages", "", "path to file containing IRC client messages to use")
	f_markov        = flag.Bool("markov", true, "use markov chains")
	f_httpCookies   = flag.Bool("httpcookies", false, "enable cookie jar in http/https clients")
	f_httpUserAgent = flag.String("http-user-agent", "", "set a custom user-agent string")
	f_ftp           = flag.Bool("ftp", false, "enable ftp service")
	f_ftps          = flag.Bool("ftps", false, "enable ftp (TLS) service")
	f_ssh           = flag.Bool("ssh", false, "enable ssh service")
	f_smtp          = flag.Bool("smtp", false, "enable smtp service")
	f_smtpUser      = flag.String("smtpuser", "", "specify a particular user to send email to for the given domain, otherwise random")
	f_smtpTls       = flag.Bool("smtptls", true, "enable or disable sending mail with TLS")
	f_smtpmail      = flag.String("smtpmail", "", "send email from a given file instead of the builtin email corpus")
	f_mean          = flag.Duration("u", time.Duration(1000*time.Millisecond), "mean time between actions")
	f_stddev        = flag.Duration("s", time.Duration(0), "standard deviation between actions")
	f_min           = flag.Duration("min", time.Duration(0), "minimum time allowable for events")
	f_max           = flag.Duration("max", time.Duration(60000*time.Millisecond), "maximum time allowable for events")
	f_v4            = flag.Bool("ipv4", true, "use IPv4. Can be used together with -ipv6")
	f_v6            = flag.Bool("ipv6", true, "use IPv6. Can be used together with -ipv4")
	f_report        = flag.Duration("report", time.Duration(10*time.Second), "time between reports, set to 0 to disable")
	f_httpTLSCert   = flag.String("httptlscert", "", "file containing public certificate for TLS")
	f_httpTLSKey    = flag.String("httptlskey", "", "file containing private key for TLS")
	f_tlsVersion    = flag.String("tlsversion", "", "Select a TLS version for the client: tls1.0, tls1.1, tls1.2")

	// See main for registering with flag
	f_httpImageSize = DefaultFileSize
	f_ftpFileSize   = DefaultFTPFileSize

	hosts map[string]string
	keys  []string
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

	// Add non-builtin flag type
	flag.Var(&f_httpImageSize, "httpimagesize", "size of image to serve in http/https pages (optional suffixes: B, KB, MB. default: MB)")
	flag.Var(&f_ftpFileSize, "ftpfilesize", "size of image file to serve in ftp/ftps RECV requests (optional suffixes: B, KB, MB. default: MB)")

	flag.Parse()

	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, syscall.SIGINT)

	log.Init()

	dns := false
	if *f_dns || *f_dnsv4 || *f_dnsv6 {
		dns = true
	}

	// make sure at least one service is enabled

	if !dns && !*f_http && !*f_https && !*f_irc && !*f_ftp && !*f_ftps && !*f_ssh && !*f_smtp {
		log.Fatalln("no enabled services")
	}

	// make sure we have both a cert & a key if the specified one
	if (*f_httpTLSCert != "" && *f_httpTLSKey == "") || (*f_httpTLSCert == "" && *f_httpTLSKey != "") {
		log.Fatalln("must provide both TLS cert & private key")
	}

	// make sure mean and variance are > 0
	if *f_mean < 0 || *f_stddev < 0 || *f_min < 0 || *f_max < 0 {
		log.Fatalln("mean, standard deviation, min, and max must be > 0")
	}

	var err error
	hosts, err = parseHosts(flag.Args())
	if err != nil {
		log.Fatalln(err)
	}
	if len(hosts) == 0 && !*f_serve {
		log.Fatalln("no hosts specified")
	}
	for k, _ := range hosts {
		keys = append(keys, k)
	}
	log.Debugln("hosts: ", hosts)

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
	if dns {
		if *f_serve {
			go dnsServer()
		} else {
			go dnsClient()
		}
	}
	if *f_http {
		if *f_serve {
			go httpServer(protocol)
		} else {
			go httpClient(protocol)
		}
	}
	if *f_https {
		if *f_serve {
			go httpTLSServer(protocol)
		} else {
			go httpTLSClient(protocol)
		}
	}
	if *f_irc {
		if *f_serve {
			go ircServer()
		} else {
			go ircClient()
		}
	}
	if *f_ftp {
		if *f_serve {
			go ftpServer()
		} else {
			go ftpClient()
		}
	}
	if *f_ftps {
		if *f_serve {
			go ftpsServer()
		} else {
			go ftpsClient()
		}
	}
	if *f_ssh {
		if *f_serve {
			go sshServer(protocol)
		} else {
			go sshClient(protocol)
		}
	}
	if *f_smtp {
		if *f_serve {
			go smtpServer(protocol)
		} else {
			go smtpClient(protocol)
		}
	}
	<-sig
}
