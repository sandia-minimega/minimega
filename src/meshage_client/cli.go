package main

import (
	"bytes"
	"fmt"
	"goreadline"
	log "minilog"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

var cli_commands map[string]*command

type command struct {
	Call func(args []string) (string, error)
	Help string
}

func init() {
	cli_commands = map[string]*command{
		"degree": &command{
			Call: func(args []string) (string, error) {
				switch len(args) {
				case 0:
					return fmt.Sprintf("%d", n.Degree()), nil
				case 1:
					i, err := strconv.Atoi(args[0])
					if err != nil {
						return "", err
					}
					n.SetDegree(uint(i))
					return "", nil
				}
				return "", fmt.Errorf("degree takes 0 or 1 arguments")
			},
			Help: "view or set the connection degree",
		},

		"dial": &command{
			Call: func(args []string) (string, error) {
				if len(args) != 1 {
					return "", fmt.Errorf("dial takes one argument")
				}
				err := n.Dial(args[0])
				return "", err
			},
			Help: "dial a host",
		},

		"dot": &command{
			Call: func(args []string) (string, error) {
				if len(args) != 1 {
					return "", fmt.Errorf("dot takes one argument")
				}
				f, err := os.Create(args[0])
				if err != nil {
					return "", err
				}

				d := n.Dot()
				f.WriteString(d)
				f.Close()
				return "", nil
			},
			Help: "write out a graphviz dot file of the mesh",
		},

		"help": &command{
			Call: func(args []string) (string, error) {
				var sorted []string
				for c, _ := range cli_commands {
					sorted = append(sorted, c)
				}
				sort.Strings(sorted)
				w := new(tabwriter.Writer)
				var buf bytes.Buffer
				w.Init(&buf, 0, 8, 0, '\t', 0)
				for _, c := range sorted {
					fmt.Fprintln(w, c, "\t", ":\t", cli_commands[c].Help, "\t")
				}
				w.Flush()
				return buf.String(), nil
			},
			Help: "help",
		},

		"status": &command{
			Call: func(args []string) (string, error) {
				mesh := n.Mesh()
				degree := n.Degree()
				nodes := len(mesh)
				host, err := os.Hostname()
				if err != nil {
					return "", err
				}
				clients := len(mesh[host])
				ret := fmt.Sprintf("mesh size %d\ndegree %d\nclients connected to this node: %d", nodes, degree, clients)
				return ret, nil
			},
			Help: "return statistics on the current mesh",
		},

		"list": &command{
			Call: func(args []string) (string, error) {
				mesh := n.Mesh()
				var ret string
				for k, v := range mesh {
					ret += fmt.Sprintf("%s\n", k)
					for _, x := range v {
						ret += fmt.Sprintf(" |--%s\n", x)
					}
				}
				return ret, nil
			},
			Help: "print the adjacency list of the mesh",
		},

		"hangup": &command{
			Call: func(args []string) (string, error) {
				if len(args) != 1 {
					return "", fmt.Errorf("hangup takes one argument")
				}
				err := n.Hangup(args[0])
				if err != nil {
					return "", err
				}
				return "", nil
			},
			Help: "hang up on a specified client",
		},

		"broadcast": &command{
			Call: func(args []string) (string, error) {
				if len(args) != 1 {
					return "", fmt.Errorf("broadcast takes one argument")
				}
				n.Broadcast(args[0])
				return "", nil
			},
			Help: "broadcast a message to other nodes",
		},

		"send": &command{
			Call: func(args []string) (string, error) {
				if len(args) < 2 {
					return "", fmt.Errorf("set takes at least two arguments")
				}
				n.Send(args[0:len(args)-1], args[len(args)-1])
				return "", nil
			},
			Help: "set send a message to one or more nodes. last argument is the message, all preceeding arguments are node names",
		},
	}
}

func cli() {
	for {
		prompt := "meshage: "
		line, err := goreadline.Rlwrap(prompt)
		if err != nil {
			break // EOF
		}
		log.Debug("got from stdin:", line)
		f := strings.Fields(string(line))
		if len(f) == 0 {
			continue
		}
		var args []string
		if len(f) > 1 {
			args = f[1:]
		}

		c := cli_commands[f[0]]
		if c != nil {
			ret, err := c.Call(args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}
			fmt.Println(ret)
		} else {
			fmt.Fprintf(os.Stderr, "invalid command\n")
		}
	}
}
