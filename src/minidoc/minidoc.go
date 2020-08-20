// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build !appengine

package main

import (
	"flag"
	log "minilog"
	"net/http"
	"os"
	"present"
	"strings"
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

	port := os.Getenv("PORT")
	if port == "" {
		port = *f_server
	} else {
		port = ":" + port
	}

	log.Fatalln(http.ListenAndServe(port, nil))
}
