// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main //THIS IS BRIAN's FILE

import (
	"bytes"
	"fmt"
	"html"
	"minicli"
	log "minilog"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const (
	GUI_PORT = 9526
	//#WEB : Will need these lines  when web.go is phased out
	newdefaultNoVNC string = "/opt/minimega/misc/novnc"
	newdefaultD3    string = "/opt/minimega/misc/d3"
	D3_SETUP               = `<meta charset="utf-8">
				  <style>
				  .chart div {
				    font: 10px sans-serif;
				    background-color: steelblue;
				    text-align: right;
				    padding: 3px;
				    margin: 1px;
				    color: white;
			          }
				  </style>
				  <div class="chart"></div>
				  <script src="/gui/d3/d3.v3.min.js">
				  </script>`
)

var (
	guiRunning bool
)

var guiCLIHandlers = []minicli.Handler{
	{ // gui
		HelpShort: "start the minimega GUI",
		HelpLong: `
Launch the GUI webserver

This command requires access to an installation of novnc. By default minimega
looks in /opt/minimega/misc/novnc. To set a different path, invoke:

	gui novnc <path to novnc>

To start the webserver on a specific port, issue the web command with the port:

	gui 9526

9526 is the default port.`,
		Patterns: []string{
			"gui [port]",
			"gui novnc <path to novnc> [port]",
		},
		Call: wrapSimpleCLI(cliGUI),
	},
}

func init() {
	registerHandlers("gui", guiCLIHandlers)
}

func cliGUI(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	port := fmt.Sprintf(":%v", GUI_PORT)
	if c.StringArgs["port"] != "" {
		// Check if port is an integer
		p, err := strconv.Atoi(c.StringArgs["port"])
		if err != nil {
			resp.Error = fmt.Sprintf("'%v' is not a valid port", c.StringArgs["port"])
			return resp
		}

		port = fmt.Sprintf(":%v", p)
	}

	noVNC := newdefaultNoVNC
	d3 := newdefaultD3
	if c.StringArgs["path"] != "" {
		noVNC = c.StringArgs["path"]
	}

	if guiRunning {
		resp.Error = "gui is already running"
	} else {
		go guiStart(port, noVNC, d3)
	}

	return resp
}

func guiStart(port, noVNC string, d3 string) {
	guiRunning = true
	http.HandleFunc("/gui/", guiRoot)
	http.Handle("/gui/novnc/", http.StripPrefix("/gui/novnc/", http.FileServer(http.Dir(noVNC))))
	http.Handle("/gui/d3/", http.StripPrefix("/gui/d3/", http.FileServer(http.Dir(d3))))
	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Error("guiStart: %v", err)
	}
	guiRunning = false
}

func guiRoot(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimSpace(r.URL.String())
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	fields := strings.Split(url, "/")
	fields = fields[1 : len(fields)-1]
	urlLen := len(fields)
	switch urlLen {
	case 1:
		w.Write([]byte(guiHome()))
	case 2: // "/gui/"
		if fields[1] == "stats" {
			w.Write([]byte(guiStats()))
		} else if fields[1] == "all" {
			w.Write([]byte(guiAllVMs()))
		} else if fields[1] == "map" {
			w.Write([]byte(guiMapVMs()))
		} else {
			w.Write([]byte(guiHosts()))
		}
	case 3: // "/gui/vnc/<host>"
		w.Write([]byte(guiHostVMs(fields[2])))
	case 4:
		if fields[1] == "ws" { // "/gui/ws/<host>/<port>"
			vncWsHandler(w, r)
		} else { // "/gui/vnc/<host>/<port>"
			title := html.EscapeString(fields[2] + ":" + fields[3]) //change to vm NAME
			path := fmt.Sprintf("/gui/novnc/vnc_auto.html?title=%v&path=gui/ws/%v/%v", title, fields[2], fields[3])
			http.Redirect(w, r, path, http.StatusTemporaryRedirect)
		}
	default:
		http.NotFound(w, r)
	}
}
func guiHome() string {
	var homebody bytes.Buffer
	//htmlformat := `<html>
	//
	//		       <head>
	//		       <title>%s</title>
	//		       </head>
	//		       <body>
	//		       %s<br>
	//		       %s
	//		       <script>
	//		       var data = [%s];
	//		       var x = d3.scale.linear()
	//		         .domain([0,d3.max(data)])
	//			 .range([0,420]);
	//		       d3.select(".chart")
	//		         .selectAll("div")
	//			 .data(data)
	//		       .enter().append("div")
	//		         .style("width", function(d) { return x(d) + "px"; })
	//			 .text(function(d) { return d; });
	//		       </script>
	//		       </body>
	//		       </html>`
	htmlformat := `<html>
                       <head>
                       <title>%s</title>
                       </head>
                       <body>
                       %s<br>
                       </body>
                       </html>`
	fmt.Fprintf(&homebody, "<a href=\"vnc\">Host List</a>")
	fmt.Fprintf(&homebody, "<br><a href=\"all\">All VMs</a>")
	fmt.Fprintf(&homebody, "<br><a href=\"stats\">Host Stats</a>")
	fmt.Fprintf(&homebody, "<br><a href=\"map\">VM Map(concept)</a>")
	//	data := `4,8,15,16,23,43`
	//return fmt.Sprintf(htmlformat, "Minimega GUI", D3_SETUP, homebody.String(), data)
	return fmt.Sprintf(htmlformat, "Minimega GUI", homebody.String())
}
func guiMapVMs() string {
	htmlformat := `<html>
                       <head>
                       <title>%s</title>
                       </head>
                       <body>
		       <h1>For proof of d3 concept only:</h1><br>
                       %s<br>
                       <script>
                       var data = [%s]; 
                       var x = d3.scale.linear() 
                         .domain([0,d3.max(data)])
                         .range([0,420]); 
                       d3.select(".chart")
                         .selectAll("div")
                         .data(data)
                       .enter().append("div")
                         .style("width", function(d) { return x(d) + "px"; })
                         .text(function(d) { return d; }); 
                       </script>
                       </body>
                       </html>`
	data := `4,8,15,16,23,43`
	return fmt.Sprintf(htmlformat, "VM Map", D3_SETUP, data)

}
func guiStats() string {
	stats := []string{}
	format := `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`

	cmdhost, err := minicli.CompileCommand("host") //mesh send all host
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	respHostChan := runCommand(cmdhost, false)

	g := <-respHostChan
	ga := g[0].Header
	stats = append(stats, fmt.Sprintf(format, ga[0], ga[1], ga[2], ga[3], ga[4], ga[5]))

	r := g[0].Tabular
	for _, r := range r {
		stats = append(stats, fmt.Sprintf(format, r[0], r[1], r[2], r[3], r[4], r[5]))
	}
	cmdhostall, err := minicli.CompileCommand("mesh send all host") //mesh send all host
	respHostAllChan := runCommand(cmdhostall, false)

	s := <-respHostAllChan
	if len(s) != 0 {
		sa := s[0].Tabular
		for _, ra := range sa {
			stats = append(stats, fmt.Sprintf(format, ra[0], ra[1], ra[2], ra[3], ra[4], ra[5]))
		}
	}
	return fmt.Sprintf(`<html><title>Host Stats</title><body><table border=1>%s</table></body></html>`, strings.Join(stats, "\n"))
}

func guiHosts() string {
	hosts := make(map[string]int)
	// first grab our own list of hosts
	count := 0
	for _, vm := range vms.vms {
		if vm.State != VM_QUIT && vm.State != VM_ERROR {
			count++
		}
	}
	hosts[hostname] = count

	cmd, err := minicli.CompileCommand("mesh send all vm info mask id,state")
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}

	remoteRespChan := runCommand(cmd, false)

	for resps := range remoteRespChan {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			count := 0
			for _, row := range resp.Tabular {
				if row[1] != "quit" && row[1] != "error" {
					count++
				}
			}
			hosts[resp.Host] = count
		}
	}

	// sort hostnames
	var sortedHosts []string
	for h, _ := range hosts {
		sortedHosts = append(sortedHosts, h)
	}
	sort.Strings(sortedHosts)
	var totalvms int
	var body bytes.Buffer
	for _, h := range sortedHosts {
		fmt.Fprintf(&body, "<a href=\"/gui/vnc/%v\">%v</a> (%v)<br>\n", h, h, hosts[h])
		totalvms += hosts[h]
	}
	fmt.Fprintf(&body, "<br>Total VMs: (%v)", totalvms)
	return fmt.Sprintf(`<html><title>Host List</title><body>%s</body></html>`, body.String())
}
func guiAllVMs() string {
	var resp chan minicli.Responses
	var respAll chan minicli.Responses
	mask := "id,host,name,state,memory,vcpus,disk,initrd,kernel,cdrom,mac,ip,vlan,append"
	cmdLocal, err := minicli.CompileCommand("vm info mask " + mask)
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	cmdRemote, err := minicli.CompileCommand(fmt.Sprintf("mesh send all vm info mask %v", mask))
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	resp = runCommand(cmdLocal, false)
	respAll = runCommand(cmdRemote, false)

	info := []string{}
	g := <-resp
	ga := g[0].Header
	if len(info) == 0 {
		header := `<tr>`
		for _, h := range ga {
			header += `<td>` + h + `</td>`
		}
		header += `</tr>`
		info = append(info, header)
	}

	r := g[0].Tabular
	for _, r := range r {
		id, err := strconv.Atoi(r[0])
		if err != nil {
			log.Errorln(err)
			return err.Error()
		}

		format := `<tr><td>%v</td><td>%v</td><td><a href="/gui/vnc/%v/%v">%v</a></td>`
		tl := fmt.Sprintf(format, id, r[1], r[1], 5900+id, r[2])
		for _, entry := range r[3:] {
			tl += `<td>` + entry + `</td>`
		}
		tl += `</tr>`
		info = append(info, tl)
	}
	sa := <-respAll
	if len(sa) != 0 {
		s := sa[0].Tabular
		if len(s) != 0 {
			for _, s := range s {
				id, err := strconv.Atoi(s[0])
				if err != nil {
					log.Errorln(err)
					return err.Error()
				}

				format := `<tr><td>%v</td><td>%v</td><td><a href="/gui/vnc/%v/%v">%v</a></td>`
				tl := fmt.Sprintf(format, id, s[1], s[1], 5900+id, s[2])
				for _, entry := range s[3:] {
					tl += `<td>` + entry + `</td>`
				}
				tl += `</tr>`
				info = append(info, tl)
			}
		}
	}
	return fmt.Sprintf(`<html><title>All VMs</title><body><table border=1>%s</table></body></html>`, strings.Join(info, "\n"))
}

func guiHostVMs(host string) string {
	var respChan chan minicli.Responses

	mask := "id,name,state,memory,vcpus,disk,initrd,kernel,cdrom,mac,ip,vlan,append"
	cmdLocal, err := minicli.CompileCommand(fmt.Sprintf("vm info mask %v", mask))
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}

	cmdRemote, err := minicli.CompileCommand(fmt.Sprintf("mesh send %v vm info mask %v", host, mask))
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}

	if host == hostname {
		respChan = runCommand(cmdLocal, false)
	} else {
		respChan = runCommand(cmdRemote, false)
	}

	lines := []string{}

	for resps := range respChan {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			// If we're the first response, we'll output the Header too.
			if len(lines) == 0 {
				header := `<tr>`
				for _, h := range resp.Header {
					header += `<td>` + h + `</td>`
				}
				header += `</tr>`
				lines = append(lines, header)
			}

			for _, row := range resp.Tabular {
				if row[2] != "error" && row[2] != "quit" {
					id, err := strconv.Atoi(row[0])
					if err != nil {
						log.Errorln(err)
						return err.Error()
					}
					format := `<tr><td>%v</td><td><a href="/gui/vnc/%v/%v">%v</a></td><td>%s</td>`
					tl := fmt.Sprintf(format, id, host, 5900+id, row[1], row[2])
					for _, entry := range row[3:] {
						tl += `<td>` + entry + `</td>`
					}
					tl += `</tr>`
					lines = append(lines, tl)
				}
			}
		}
	}

	if len(lines) == 0 {
		return "no VMs found"
	}

	return fmt.Sprintf(`<html><title>VM list</title><body><table border=1>%s</table></body></html>`, strings.Join(lines, "\n"))
}
