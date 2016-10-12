// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	"os"
	"ron"
	"strconv"
	"strings"
)

var filter *ron.Filter

var cliHandlers = []minicli.Handler{
	{
		HelpShort: "list clients",
		Patterns: []string{
			"clients",
		},
		Call: func(cmd *minicli.Command, out chan<- minicli.Responses) {
			resp := &minicli.Response{
				Host: hostname,
				Header: []string{
					"UUID", "arch", "OS", "hostname", "IPs", "MACs",
				},
			}

			for _, client := range rond.GetActiveClients() {
				row := []string{
					client.UUID,
					client.Arch,
					client.OS,
					client.Hostname,
					fmt.Sprintf("%v", client.IPs),
					fmt.Sprintf("%v", client.MACs),
				}

				resp.Tabular = append(resp.Tabular, row)
			}

			out <- minicli.Responses{resp}
		},
	},
	{
		HelpShort: "set filter for subsequent commands",
		Patterns: []string{
			"filter [filter]",
			"<clear,> filter",
		},
		Call: func(cmd *minicli.Command, out chan<- minicli.Responses) {
			resp := &minicli.Response{
				Host: hostname,
			}

			arg := cmd.StringArgs["filter"]

			if cmd.BoolArgs["clear"] {
				filter = nil
			} else if cmd.StringArgs["filter"] == "" {
				resp.Response = fmt.Sprintf("%#v", filter)
			} else if f, err := parseFilter(arg); err != nil {
				resp.Error = err.Error()
			} else {
				filter = f
			}

			out <- minicli.Responses{resp}
		},
	},
	{
		HelpShort: "run a command",
		Patterns: []string{
			"<exec,> <command>...",
			"<bg,> <command>...",
			"<shell,> <command>...",
		},
		Call: func(cmd *minicli.Command, out chan<- minicli.Responses) {
			resp := &minicli.Response{
				Host: hostname,
			}

			id := rond.NewCommand(&ron.Command{
				Command:    cmd.ListArgs["command"],
				Filter:     filter,
				Background: cmd.BoolArgs["bg"],
			})

			if cmd.BoolArgs["bg"] || cmd.BoolArgs["exec"] {
				resp.Response = strconv.Itoa(id)
				out <- minicli.Responses{resp}
			} else {
				// wait for response

				// TODO
			}
		},
	},
	{
		HelpShort: "list processes",
		Patterns: []string{
			"processes",
		},
		Call: func(cmd *minicli.Command, out chan<- minicli.Responses) {
			resp := &minicli.Response{
				Host: hostname,
				Header: []string{
					"UUID", "pid", "command",
				},
			}

			for _, client := range rond.GetActiveClients() {
				for _, proc := range client.Processes {
					row := []string{
						client.UUID,
						strconv.Itoa(proc.PID),
						fmt.Sprintf("%v", proc.Command),
					}

					resp.Tabular = append(resp.Tabular, row)
				}
			}

			out <- minicli.Responses{resp}
		},
	},
	{
		HelpShort: "kill PID",
		Patterns: []string{
			"kill <PID>",
		},
		Call: func(cmd *minicli.Command, out chan<- minicli.Responses) {
			resp := &minicli.Response{
				Host: hostname,
			}

			pid, err := strconv.Atoi(cmd.StringArgs["PID"])
			if err != nil {
				resp.Error = err.Error()
				out <- minicli.Responses{resp}
				return
			}

			rond.NewCommand(&ron.Command{
				PID:    pid,
				Filter: filter,
			})

			out <- minicli.Responses{resp}
		},
	},
	{
		HelpShort: "kill by name",
		Patterns: []string{
			"killall <name>",
		},
		Call: func(cmd *minicli.Command, out chan<- minicli.Responses) {
			resp := &minicli.Response{
				Host: hostname,
			}

			rond.NewCommand(&ron.Command{
				KillAll: cmd.StringArgs["name"],
				Filter:  filter,
			})

			out <- minicli.Responses{resp}
		},
	},
	{
		HelpShort: "send files",
		Patterns: []string{
			"send <name>",
		},
		Call: func(cmd *minicli.Command, out chan<- minicli.Responses) {
			resp := &minicli.Response{
				Host: hostname,
			}

			out <- minicli.Responses{resp}
		},
	},
	{
		HelpShort: "get files",
		Patterns: []string{
			"get <name>",
		},
		Call: func(cmd *minicli.Command, out chan<- minicli.Responses) {
			resp := &minicli.Response{
				Host: hostname,
			}

			out <- minicli.Responses{resp}
		},
	},
	{
		HelpShort: "quit",
		Patterns: []string{
			"quit",
		},
		Call: func(cmd *minicli.Command, out chan<- minicli.Responses) {
			os.Exit(0)
		},
	},
}

func parseFilter(s string) (*ron.Filter, error) {
	filter := &ron.Filter{}

	if s == "" {
		return nil, nil
	}

	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed id=value pair: %v", s)
	}

	switch strings.ToLower(parts[0]) {
	case "uuid":
		filter.UUID = strings.ToLower(parts[1])
	case "hostname":
		filter.Hostname = parts[1]
	case "arch":
		filter.Arch = parts[1]
	case "os":
		filter.OS = parts[1]
	case "ip":
		filter.IP = parts[1]
	case "mac":
		filter.MAC = parts[1]
	case "tag":
		// Explicit filter on tag
		parts = parts[1:]
		fallthrough
	default:
		// Implicit filter on a tag
		if filter.Tags == nil {
			filter.Tags = make(map[string]string)
		}

		// Split on `=` or `:` -- who cares if they did `tag=foo=bar`,
		// `tag=foo:bar` or `foo=bar`. `=` takes precedence.
		if strings.Contains(parts[0], "=") {
			parts = strings.SplitN(parts[0], "=", 2)
		} else if strings.Contains(parts[0], ":") {
			parts = strings.SplitN(parts[0], ":", 2)
		}

		if len(parts) == 1 {
			filter.Tags[parts[0]] = ""
		} else if len(parts) == 2 {
			filter.Tags[parts[0]] = parts[1]
		}
	}

	return filter, nil
}
