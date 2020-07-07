// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

// apigen creates the api documentation by invoking minimega's JSON API output
// (-cli), and applying the data to a minidoc api template. It is expected to
// be invoked by the build script.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	f_template = flag.String("template", "", "api generation template")
	f_bin      = flag.String("bin", "", "binary to extract json api doc from")
	f_sections = flag.String("sections", "", "CSV of section names")
)

type apigen struct {
	Date     string
	Sections map[string][]*minicli.Handler
}

func main() {
	flag.Parse()

	log.Init()

	if *f_bin == "" {
		log.Fatalln("must specify binary")
	}

	if *f_template == "" {
		log.Fatalln("must specify template")
	}

	log.Debug("using binary: %v", *f_bin)
	log.Debug("using doc template: %v", *f_template)

	// invoke minimega and get the doc json
	doc, err := exec.Command(*f_bin, "-cli").Output()
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
		Date:     fmt.Sprintf("%v %v %v", day, month, year),
		Sections: map[string][]*minicli.Handler{},
	}

	// sort handlers
	sort.Slice(handlers, func(i, j int) bool { return handlers[i].SharedPrefix < handlers[j].SharedPrefix })

	// collapse handlers based on SharedPrefix
	matched := 0
	for i := range handlers {
		j := i - matched

		if j > 0 && handlers[j].SharedPrefix == handlers[j-1].SharedPrefix {
			handlers[j-1].Patterns = append(handlers[j-1].Patterns, handlers[j].Patterns...)
			handlers[j-1].HelpShort += "\n" + handlers[j].HelpShort
			handlers[j-1].HelpLong += "\n" + handlers[j].HelpLong

			// delete matched handler
			handlers = append(handlers[:j], handlers[j+1:]...)
			matched++
		}
	}

	for _, s := range strings.Split(*f_sections, ",") {
		matches := []*minicli.Handler{}

		for i := range handlers {
			j := i - len(matches)
			p := handlers[j].SharedPrefix

			if strings.HasPrefix(p, "clear") {
				p = strings.TrimPrefix(p, "clear ")
			}

			if strings.HasPrefix(p, s) {
				matches = append(matches, handlers[j])

				// delete matched handler
				handlers = append(handlers[:j], handlers[j+1:]...)
			}
		}

		log.Info("found %v matches for section %v", len(matches), s)

		if len(matches) == 0 {
			log.Warn("no matches found for section: %v", s)
		}

		api.Sections[s] = matches
	}

	// add section for unmatched handlers
	if len(handlers) > 0 {
		api.Sections[""] = handlers
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
