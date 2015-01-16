// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"html"
	"minicli"
	log "minilog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	GOVNC_PORT          = 9001
	defaultNoVNC string = "misc/novnc"
)

var (
	webRunning bool
)

var webCLIHandlers = []minicli.Handler{
	{ // web
		HelpShort: "start the minimega web interface",
		HelpLong: `
Launch a webserver that allows you to browse the connected minimega hosts and
VMs, and connect to any VM in the pool.

This command requires access to an installation of novnc. By default minimega
looks in 'pwd'/misc/novnc. To set a different path, invoke:

	web novnc <path to novnc>

To start the webserver on a specific port, issue the web command with the port:

	web 7000

9001 is the default port.`,
		Patterns: []string{
			"web [port]",
			"web novnc <path to novnc> [port]",
		},
		Call: wrapSimpleCLI(cliWeb),
	},
}

func init() {
	registerHandlers("web", webCLIHandlers)
}

// TODO: I changed how this command works to make it more intuitive (at least
// for me). I removed the ability to configure/clear novnc independent of
// starting the web server. There currently isn't a way to stop
// http.ListenAndServe so "clear web" doesn't make sense.
func cliWeb(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	port := fmt.Sprintf(":%v", GOVNC_PORT)
	if c.StringArgs["port"] != "" {
		// Check if port is an integer
		p, err := strconv.Atoi(c.StringArgs["port"])
		if err != nil {
			resp.Error = fmt.Sprintf("'%v' is not a valid port", c.StringArgs["port"])
			return resp
		}

		port = fmt.Sprintf(":%v", p)
	}

	noVNC := defaultNoVNC
	if c.StringArgs["path"] != "" {
		noVNC = c.StringArgs["path"]
	}

	if webRunning {
		resp.Error = "web interface is already running"
	} else {
		go webStart(port, noVNC)
	}

	return resp
}

func webStart(port, noVNC string) {
	webRunning = true
	http.HandleFunc("/vnc/", vncRoot)
	http.Handle("/novnc/", http.StripPrefix("/novnc/", http.FileServer(http.Dir(noVNC))))
	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Error("webStart: %v", err)
	}
	webRunning = false
}

func vncRoot(w http.ResponseWriter, r *http.Request) {
	// there are four things we can serve:
	// 	1. "/" - show the list of hosts
	//	2. "/<host>" - show the list of host VMs
	//	3. "/<host>/<value>" - redirect to the novnc html with a path
	//	4. "/ws/<host>/<value>" - create a tunnel
	url := r.URL.String()
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	fields := strings.Split(url, "/")
	switch len(fields) {
	case 3: // "/"
		w.Write([]byte(webHosts()))
	case 4: // "/<host>"
		w.Write([]byte(webHostVMs(fields[2])))
	case 5: // "/<host>/<port>"
		title := html.EscapeString(fields[2] + ":" + fields[3])
		path := fmt.Sprintf("/novnc/vnc_auto.html?title=%v&path=vnc/ws/%v/%v", title, fields[2], fields[3])
		http.Redirect(w, r, path, http.StatusTemporaryRedirect)
	case 6: // "/ws/<host>/<port>"
		vncWsHandler(w, r)
	default:
		http.NotFound(w, r)
	}
}

func webHosts() string {
	hosts := make(map[string]int)
	// first grab our own list of hosts
	host, err := os.Hostname()
	if err != nil {
		log.Errorln(err)
		teardown()
	}
	count := 0
	for _, vm := range vms.vms {
		if vm.State != VM_QUIT && vm.State != VM_ERROR {
			count++
		}
	}
	hosts[host] = count

	// get a list of the other hosts on the network
	cmd := cliCommand{
		Args: []string{"hostname"},
	}
	resp := meshageBroadcast(cmd)
	if resp.Error != "" {
		log.Errorln(resp.Error)
		return ""
	}

	otherHosts := strings.Fields(resp.Response)

	for _, h := range otherHosts {
		// get a list of vms from that host
		cmd := cliCommand{
			Args: []string{h, "vm_info", "[id,state]"},
		}
		resp := meshageSet(cmd)
		if resp.Error != "" {
			log.Errorln(resp.Error)
			continue // don't error out if just one host fails us
		}

		lines := strings.Split(resp.Response, "\n")
		count := 0
		for _, l := range lines[1:] {
			f := strings.Fields(l)
			if len(f) == 3 {
				if f[2] != "quit" && f[2] != "error" {
					count++
				}
			}
		}
		hosts[h] = count
	}
	if len(hosts) == 0 {
		return "no hosts found"
	}
	body := ""

	// sort hostnames
	var sortedHosts []string
	for h, _ := range hosts {
		sortedHosts = append(sortedHosts, h)
	}
	sort.Strings(sortedHosts)

	for _, h := range sortedHosts {
		body += fmt.Sprintf("<a href=\"/vnc/%v\">%v</a> (%v)<br>\n", h, h, hosts[h])
	}
	return body
}

// this whole block is UGLY, please rewrite
func webHostVMs(host string) string {
	localhost, err := os.Hostname()
	if err != nil {
		log.Errorln(err)
		teardown()
	}

	var d string

	if localhost == host {
		//cmd := cliCommand{}
		r := cliResponse{} // TODO: vms.info(cmd)
		if r.Error != "" {
			log.Errorln(r.Error)
			return r.Error
		}
		d = r.Response
	} else {
		cmd := cliCommand{
			Args: []string{host, "vm_info"},
		}
		r := meshageSet(cmd)
		if r.Error != "" {
			log.Errorln(r.Error)
			return r.Error
		}
		d = r.Response
	}

	body := `
	<html>
	<body>
	<table border=1>
	`
	lines := strings.Split(d, "\n")
	if len(lines) < 2 {
		return "no VMs found"
	}
	f := strings.Fields(lines[0])
	body += "<tr>"
	for _, x := range f {
		if x != "|" {
			body += "<td>" + x + "</td>"
		}
	}
	body += "</tr>"
	for _, l := range lines[1:] {
		if !strings.Contains(l, "error") && !strings.Contains(l, "quit") {
			f := strings.Fields(l)
			if len(f) == 0 { // skip blank lines
				continue
			}
			val, err := strconv.Atoi(f[0])
			if err != nil {
				log.Errorln(err)
				return err.Error()
			}
			body += "<tr>"
			collect := fmt.Sprintf("<a href=\"/vnc/%v/%v\">%v</a>", host, 5900+val, f[0])
			for _, x := range f[1:] {
				if x == "|" {
					body += fmt.Sprintf("<td>%v</td>", collect)
					collect = ""
				} else {
					collect += " " + x
				}
			}
			body += fmt.Sprintf("<td>%v</td></tr>", collect)
		}
	}
	body += "</body></html>"
	return body

}
