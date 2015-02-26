// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// apigen creates the api documentation by invoking minimega's JSON API output
// (-cli), and applying the data to a minidoc api template. It is expected to
// be invoked by the build script.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"minicli"
	log "minilog"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

var (
	f_template = flag.String("template", "doc/content_templates/api.template", "api generation template")
	f_minimega = flag.String("minimega", "bin/minimega", "minimega binary to extract json api doc from")
	f_loglevel = flag.String("level", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
)

type apigen struct {
	Date     string
	Builtins []*minicli.Handler
	Mesh     []*minicli.Handler
	VM       []*minicli.Handler
	Host     []*minicli.Handler
}

func main() {
	flag.Parse()

	logSetup()

	log.Debug("using minimega: %v", *f_minimega)
	log.Debug("using doc template: %v", *f_template)

	// invoke minimega and get the doc json
	doc, err := exec.Command(*f_minimega, "-cli").Output()
	if err != nil {
		log.Fatalln(err)
	}
	log.Debug("got doc: %v", string(doc))

	// decode the JSON for our template
	var handlers []*minicli.Handler
	err = json.Unmarshal(doc, &handlers)
	if err != nil {
		log.Fatalln(err)
	}

	// populate the apigen date for the template
	year, month, day := time.Now().Date()
	api := apigen{
		Date: fmt.Sprintf("%v %v %v", day, month, year),
	}

	// populate the major sections for the template
	for _, v := range handlers {
		var p string
		if strings.HasPrefix(v.SharedPrefix, "clear") {
			p = strings.TrimPrefix(v.SharedPrefix, "clear ")
		} else {
			p = v.SharedPrefix
		}
		if strings.HasPrefix(p, ".") {
			api.Builtins = append(api.Builtins, v)
		} else if strings.HasPrefix(p, "mesh") {
			api.Mesh = append(api.Mesh, v)
		} else if strings.HasPrefix(p, "vm") {
			api.VM = append(api.VM, v)
		} else {
			api.Host = append(api.Host, v)
		}
	}

	// run the template and print to stdout
	var out bytes.Buffer
	t, err := template.ParseFiles(*f_template)
	if err != nil {
		log.Fatalln(err)
	}
	err = t.Execute(&out, api)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(out.String())
}
