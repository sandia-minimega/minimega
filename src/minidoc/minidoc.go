// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build !appengine

package main

import (
	"flag"
	log "minilog"
	"net/http"
	"present"
	"strings"
)

var (
	f_server   = flag.String("server", ":9003", "HTTP server \"host:port\"")
	f_root     = flag.String("root", "doc/content/", "HTTP root directory")
	f_base     = flag.String("base", "doc/template/", "base path for static content and templates")
	f_exec     = flag.Bool("exec", false, "allow minimega commands")
	f_minimega = flag.String("minimega", "/tmp/minimega", "path to minimega base directory")
)

func main() {
	flag.Parse()

	if !strings.HasSuffix(*f_minimega, "/") {
		*f_minimega += "/"
	}

	log.Init()

	err := initTemplates(*f_base)
	if err != nil {
		log.Fatal("failed to parse templates: %v", err)
	}

	if *f_exec {
		present.PlayEnabled = true
		http.Handle("/socket", NewSocketHandler())
	}

	log.Fatalln(http.ListenAndServe(*f_server, nil))
}
