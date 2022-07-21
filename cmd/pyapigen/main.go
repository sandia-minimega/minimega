// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/version"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// blacklist contains commands to filter out so they don't show up in the API
// this list should also include any commands you want to generate manually
var blacklist = map[string]bool{
	"help":            true,
	"namespace":       true,
	"clear namespace": true,
}

var (
	f_out = flag.String("out", "minimega.py", "output file")
)

// parseCLI runs minimega and parses the CLI output.
func parseCLI(minimega string) ([]*minicli.Handler, error) {
	data, err := exec.Command(minimega, "-cli").CombinedOutput()
	if err != nil {
		return nil, err
	}

	var handlers []*minicli.Handler
	if err := json.Unmarshal(data, &handlers); err != nil {
		return nil, err
	}

	return handlers, nil
}

func usage() {
	fmt.Printf("USAGE: %v path/to/minimega\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	handlers, err := parseCLI(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	cmds := make(map[string]*Command)
	for _, h := range handlers {
		if strings.HasPrefix(h.SharedPrefix, ".") || blacklist[h.SharedPrefix] {
			continue
		}

		// def tap()
		// def tap_create(vlan, tap=None, dhcp=None, ip=None, bridge=None)
		// def tap_delete(id)
		for _, pattern := range h.PatternItems {
			name := genName(pattern)

			if _, ok := cmds[name]; !ok {
				cmds[name] = &Command{
					Name: name,
				}
			}

			cmd := cmds[name]

			if cmd.Help == "" {
				cmd.Help = h.HelpLong
			}
			if cmd.Help == "" {
				cmd.Help = h.HelpShort
			}

			if pattern[len(pattern)-1].IsOptional() {
				// HAX: create two patterns, one with and without the optional
				// last field to simplify things later.
				v := NewVariant(pattern)
				v.Pattern = strings.Replace(v.Pattern, "[", "<", -1)
				v.Pattern = strings.Replace(v.Pattern, "]", ">", -1)
				cmd.Variants = append(cmd.Variants, v)

				pattern = pattern[:len(pattern)-1]
			}

			cmd.Variants = append(cmd.Variants, NewVariant(pattern))
		}
	}

	names := []string{}
	for v := range cmds {
		names = append(names, v)
	}

	sort.Strings(names)

	for _, cmd := range cmds {
		args := map[string]*Arg{}

		for i, v := range cmd.Variants {
			visited := map[string]bool{}

			// add any new arguments from this variant and update optional
			for _, arg := range v.Args {
				visited[arg.Name] = true

				if _, ok := args[arg.Name]; !ok {
					args[arg.Name] = &Arg{
						Name:     arg.Name,
						Optional: arg.Optional,
						Options:  arg.Options,
					}
					if i > 0 {
						args[arg.Name].Optional = true
					}
					continue
				}

				if !args[arg.Name].Optional && arg.Optional {
					args[arg.Name].Optional = true
				}
			}

			// any arg that wasn't visited is optional
			for name := range args {
				if !visited[name] {
					args[name].Optional = true
				}
			}
		}

		// reprocess the args again so that we add them in a "natural" order.
		// First add everything that is non-optional. Then, do it again for the
		// optional ones.
		for _, v := range cmd.Variants {
			for _, arg := range v.Args {
				if v := args[arg.Name]; v != nil && !v.Optional {
					cmd.Args = append(cmd.Args, *v)
					delete(args, arg.Name)
				}
			}
		}
		for _, v := range cmd.Variants {
			for _, arg := range v.Args {
				if v := args[arg.Name]; v != nil {
					cmd.Args = append(cmd.Args, *v)
					delete(args, arg.Name)
				}
			}
		}

		sort.Slice(cmd.Variants, func(i, j int) bool {
			return len(cmd.Variants[i].Args) > len(cmd.Variants[j].Args)
		})
	}

	out, err := os.Create(*f_out)
	if err != nil {
		log.Fatal(err)
	}

	var context = struct {
		Commands map[string]*Command
		Version  string
		Date     string
	}{cmds, version.Revision, time.Now().String()}

	if err := Template.Execute(out, context); err != nil {
		log.Fatal(err)
	}
}

// genName creates a name from the pattern using all literal strings.
func genName(pattern []minicli.PatternItem) string {
	parts := []string{}

	for _, item := range pattern {
		var s string

		if item.IsLiteral() {
			s = item.Text
		} else if item.IsChoice() && !item.IsOptional() && len(item.Options) == 1 {
			s = item.Options[0]
		}

		if s == "" {
			continue
		}

		parts = append(parts, strings.Replace(s, "-", "_", -1))
	}

	return strings.Join(parts, "_")
}
