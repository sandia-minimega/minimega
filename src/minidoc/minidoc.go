// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build !appengine
package main

import (
	"flag"
	"fmt"
	log "minilog"
	"net/http"
	"present"
	"strings"
)

var (
	f_port     = flag.Int("port", 9003, "HTTP port")
	f_root     = flag.String("root", "doc/content/", "HTTP root directory")
	f_base     = flag.String("base", "doc/template/", "base path for static content and templates")
	f_exec     = flag.Bool("exec", false, "allow minimega commands")
	f_loglevel = flag.String("level", "debug", "log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "log to file")
	f_minimega = flag.String("minimega", "/tmp/minimega", "path to minimega base directory")
)

func main() {
	flag.Parse()

	if !strings.HasSuffix(*f_minimega, "/") {
		*f_minimega += "/"
	}

	logSetup()

	err := initTemplates(*f_base)
	if err != nil {
		log.Fatal("failed to parse templates: %v", err)
	}

	if *f_exec {
		present.PlayEnabled = true
		http.Handle("/socket", NewSocketHandler())
	}

	host := fmt.Sprintf(":%v", *f_port)
	log.Fatalln(http.ListenAndServe(host, nil))
}
