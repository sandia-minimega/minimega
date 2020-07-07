//go:build appengine
// +build appengine

package main

import (
	"flag"
	//log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func init() {
	flag.Parse()
	initTemplates("./doc/template/")
}
