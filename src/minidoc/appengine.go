// +build appengine

package main

import (
	"flag"
	//log "minilog"
)

func init() {
	flag.Parse()
	initTemplates("./doc/template/")
}
