// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"fmt"
	"meshage"
	"minicli"
	"miniclient"
	log "minilog"
	"miniplumber"
	"os"
	"os/exec"
	"strings"
)

var (
	plumber *miniplumber.Plumber
)

var plumbCLIHandlers = []minicli.Handler{
	{ // plumb
		HelpShort: "plumb external programs with minimega, VMs, and other external programs",
		HelpLong:  ``,
		Patterns: []string{
			"plumb <src> <dst>...",
		},
		Call: wrapSimpleCLI(cliPlumbLocal),
	},
	{ // plumb
		Patterns: []string{
			"plumb",
		},
		Call: wrapBroadcastCLI(cliPlumbBroadcast),
	},
	{
		HelpShort: "reset plumber state",
		HelpLong:  ``,
		Patterns: []string{
			"clear plumb [pipeline]...",
		},
		Call: wrapBroadcastCLI(cliPlumbClear),
	},
	{ // pipe
		HelpShort: "write to and modify named pipes",
		HelpLong:  ``,
		Patterns: []string{
			"pipe",
			"pipe <pipe> <mode,> <all,round-robin,random>",
			"pipe <pipe> <log,> <true,false>",
		},
		Call: wrapBroadcastCLI(cliPipeBroadcast),
	},
	{ // pipe
		Patterns: []string{
			"pipe <pipe> <data>",
			"pipe <pipe> <via,> <command>...",
		},
		Call: wrapSimpleCLI(cliPipeLocal),
	},
	{
		HelpShort: "reset pipe state",
		HelpLong:  ``,
		Patterns: []string{
			"clear pipe [pipe]",
			"clear pipe <pipe> <mode,>",
			"clear pipe <pipe> <log,>",
			"clear pipe <pipe> <via,>",
		},
		Call: wrapBroadcastCLI(cliPipeClear),
	},
}

func plumberStart(node *meshage.Node) {
	plumber = miniplumber.New(node)
}

func cliPlumbLocal(c *minicli.Command, resp *minicli.Response) error {
	args := append([]string{c.StringArgs["src"]}, c.ListArgs["dst"]...)

	// rewrite pipes with namespace prefixes
	ns := GetNamespace()
	if ns != nil {
		for i, e := range args {
			if fields := strings.Split(e, "//"); len(fields) == 1 {
				f := fieldsQuoteEscape("\"", e)
				_, err := exec.LookPath(f[0])
				if err != nil {
					args[i] = fmt.Sprintf("%v//%v", ns, e)
				}
			}
		}
	}

	log.Debug("got plumber production: %v", args)

	return plumber.Plumb(args...)
}

func cliPlumbBroadcast(c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{"pipeline"}
	resp.Tabular = [][]string{}

	for _, v := range plumber.Pipelines() {
		if ns := GetNamespace(); ns != nil {
			if !strings.Contains(v, fmt.Sprintf("%v//", ns)) {
				continue
			}
			v = strings.Replace(v, fmt.Sprintf("%v//", ns), "", -1)
		}
		resp.Tabular = append(resp.Tabular, []string{v})
	}

	return nil
}

func cliPlumbClear(c *minicli.Command, resp *minicli.Response) error {
	if pipeline, ok := c.ListArgs["pipeline"]; ok {
		return plumber.PipelineDelete(pipeline...)
	} else {
		return plumber.PipelineDeleteAll()
	}
}

func cliPipeBroadcast(c *minicli.Command, resp *minicli.Response) error {
	pipe := c.StringArgs["pipe"]

	// rewrite the pipe with the namespace prefix, if any
	ns := GetNamespace()
	if ns != nil {
		if fields := strings.Split(pipe, "//"); len(fields) == 1 {
			pipe = fmt.Sprintf("%v//%v", ns, pipe)
		}
	}

	if c.BoolArgs["mode"] {
		var mode int
		if c.BoolArgs["all"] {
			mode = miniplumber.MODE_ALL
		} else if c.BoolArgs["round-robin"] {
			mode = miniplumber.MODE_RR
		} else if c.BoolArgs["random"] {
			mode = miniplumber.MODE_RND
		}
		plumber.Mode(pipe, mode)

		return nil
	} else if c.BoolArgs["log"] {
		if c.BoolArgs["true"] {
			plumber.Log(pipe, true)
		} else {
			plumber.Log(pipe, false)
		}
	} else {
		// get info on all named pipes
		resp.Header = []string{"name", "mode", "readers", "writers", "via", "last message"}
		resp.Tabular = [][]string{}

		for _, v := range plumber.Pipes() {
			name := v.Name()
			if ns := GetNamespace(); ns != nil {
				if !strings.Contains(name, fmt.Sprintf("%v//", ns)) {
					continue
				}
				name = strings.Replace(name, fmt.Sprintf("%v//", ns), "", -1)
			}
			resp.Tabular = append(resp.Tabular, []string{name, v.Mode(), fmt.Sprintf("%v", v.NumReaders()), fmt.Sprintf("%v", v.NumWriters()), v.GetVia(), strings.TrimSpace(v.Last())})
		}
	}

	return nil
}

func cliPipeLocal(c *minicli.Command, resp *minicli.Response) error {
	pipe := c.StringArgs["pipe"]

	// rewrite the pipe with the namespace prefix, if any
	ns := GetNamespace()
	if ns != nil {
		if fields := strings.Split(pipe, "//"); len(fields) == 1 {
			pipe = fmt.Sprintf("%v//%v", ns, pipe)
		}
	}

	if c.BoolArgs["via"] {
		plumber.Via(pipe, c.ListArgs["command"])
	} else {
		data := c.StringArgs["data"]
		plumber.Write(pipe, data)
	}
	return nil
}

func cliPipeClear(c *minicli.Command, resp *minicli.Response) error {
	pipe, ok := c.StringArgs["pipe"]

	if c.BoolArgs["mode"] {
		if !ok {
			return fmt.Errorf("no such pipe: %v", pipe)
		}
		plumber.Mode(pipe, miniplumber.MODE_ALL)
	} else if c.BoolArgs["log"] {
		if !ok {
			return fmt.Errorf("no such pipe: %v", pipe)
		}
		plumber.Log(pipe, false)
	} else if c.BoolArgs["via"] {
		if !ok {
			return fmt.Errorf("no such pipe: %v", pipe)
		}
		plumber.Via(pipe, []string{})
	} else {
		if ok {
			return plumber.PipeDelete(pipe)
		} else {
			return plumber.PipeDeleteAll()
		}
	}

	return nil
}

func pipeMMHandler() {
	pipe := *f_pipe

	log.Debug("got pipe: %v", pipe)

	// connect to the running minimega as a plumber
	mm, err := miniclient.Dial(*f_base)
	if err != nil {
		log.Fatalln(err)
	}

	r, w := mm.Pipe(pipe)
	wait := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			_, err := os.Stdout.Write(append(scanner.Bytes(), '\n'))
			if err != nil {
				log.Fatalln(err)
			}
		}
		if err := scanner.Err(); err != nil {
			log.Fatalln(err)
		}
		close(wait)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		log.Debug("writing: %v", scanner.Text())
		_, err := w.Write(append(scanner.Bytes(), '\n'))
		if err != nil {
			log.Fatalln(err)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalln(err)
	}

	// we can't just exit at this point, as there exists a race between
	// writing to the pipe and the other end reading and sending the data
	// over the command socket. Instead, we close the writer and wait until
	// the miniclient pipe handler exits for us.
	w.Close()

	<-wait
}
