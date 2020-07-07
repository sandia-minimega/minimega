// +build appengine

package main

import (
	"flag"
	//log "github.com/sandia-minimega/minimega/pkg/minilog"
)

func init() {
	flag.Parse()
	initTemplates("./doc/template/")
}
