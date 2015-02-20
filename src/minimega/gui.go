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
	newdefaultTerm  string = "/opt/minimega/misc/terminal"
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
	HTMLFRAME = `<!DOCTYPE html>
                       <head>
                       <title>Minimega GUI</title>
                       <link rel="stylesheet" type="text/css" href="d3/nav.css">
                       </head>
                       <body>
		       <nav><ul><li><a href="vnc">Host List</a></li>
                         <li><a href="all">All VMs</a></li>
                         <li><a href="stats">Host Stats</a></li>
                         <li><a href="map">VM Map(concept)</a></li>
                         <li><a href="terminal/terminal.html">Terminal(concept)</a></li>
                       </ul></nav>	
                       %s
                       </body>
                       </html>`
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
	term := newdefaultTerm
	if c.StringArgs["path"] != "" {
		noVNC = c.StringArgs["path"]
	}

	if guiRunning {
		resp.Error = "gui is already running"
	} else {
		go guiStart(port, noVNC, d3, term)
	}

	return resp
}

func guiStart(port, noVNC string, d3 string, term string) {
	guiRunning = true
	http.HandleFunc("/gui/", guiRoot)
	http.Handle("/gui/novnc/", http.StripPrefix("/gui/novnc/", http.FileServer(http.Dir(noVNC))))
	http.Handle("/gui/terminal/", http.StripPrefix("/gui/terminal/", http.FileServer(http.Dir(term))))
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
	//var homebody bytes.Buffer
	//htmlformat := `<!DOCTYPE html>
	//               <head>
	//               <title>%s</title>
	//               </head>
	//               <body>
	//		       <link rel="stylesheet" type="text/css" href="d3/nav.css">
	//                      %s<br>
	//                     </body>
	//                    </html>`
	//	navbar := `<nav><ul><li><a href="vnc">Host List</a></li>
	//		     <li><a href="all">All VMs</a></li>
	//	             <li><a href="stats">Host Stats</a></li>
	//	             <li><a href="map">VM Map(concept)</a></li>
	//	           </ul></nav>`

	//fmt.Fprintf(&homebody, "<a href=\"vnc\">Host List</a>")
	//fmt.Fprintf(&homebody, "<br><a href=\"all\">All VMs</a>")
	//fmt.Fprintf(&homebody, "<br><a href=\"stats\">Host Stats</a>")
	//fmt.Fprintf(&homebody, "<br><a href=\"map\">VM Map(concept)</a>")
	return fmt.Sprintf(HTMLFRAME, "Home") //homebody.String())
}
func guiMapVMs() string {
	//htmlformat := `<html>
	//             <head>
	//           <title>%s</title>
	//         </head>
	//       <body>
	//     <h1>For proof of d3 concept only:</h1><br>
	//   %s<br>
	//              <script>
	//            var data = [%s];
	//          var x = d3.scale.linear()
	//          .domain([0,d3.max(data)])
	//        .range([0,420]);
	//               d3.select(".chart")
	//               .selectAll("div")
	//             .data(data)
	//         .enter().append("div")
	//         .style("width", function(d) { return x(d) + "px"; })
	///              .text(function(d) { return d; });
	//         </script>
	//       </body>
	//     </html>`
	//	tableformat := `<tr><td>%s</td></tr>`
	mask := `id`
	//	tblrw := ""
	//	tl := ""
	list := getVMinfo(mask)
	//	for _, row := range list {
	//		tl := `<tr>`
	//		for _, item := range list[row] {
	//			tl += `<td>` + item + `</td>`
	//		}
	//		tl += `</tr>`
	//		tblrw += tl
	//	}
	//	vmIDs := fmt.Sprintf(`<table border=1>%s</table>`, tblrw)
	vmIDs := list
	format := `<h1>For proof of d3 concept only:</h1><br>
                       %s<br>
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
                       </script>`
	data := `4,8,15,16,23,43`
	body := fmt.Sprintf(format, D3_SETUP, vmIDs, data)
	//return fmt.Sprintf(htmlformat, "VM Map", D3_SETUP, data)
	return fmt.Sprintf(HTMLFRAME, body)
}

//func getVMinfo(mask string) string {
//	list := []string{}
//	cmdLocal, errLocal := minicli.CompileCommand(fmt.Sprintf(`vm info %v`, mask))
//	cmd, err := minicli.CompileCommand(fmt.Sprintf(`mesh send all vm info %v`, mask))
//	if err != nil {
//		// Should never happen
//		log.Fatalln(err)
//	}
//	if errLocal != nil {
//		// Should never happen
//		log.Fatalln(errLocal)
//	}
//	respChanLocal := runCommand(cmdLocal, false)
//	respChan := runCommand(cmd, false)
//	r := <-respChan
///	l := <-respChanLocal
//
//	//all:
//	if len(l) != 0 {
//		la := l[0].Tabular
//		//list = append(list, la)
//		obb := string{}
//		for _, ecah := range la {
//			for _, e := range ecah {
//				obb = append(obb, string(e))
//			}
//		}
//		list = append(list, obb)
//	}
//	if len(r) != 0 {
//		ra := r[0].Tabular
//		bob := string{}
//		for _, each := range ra {
//			for _, ea := range each {
//				bob = append(bob, string(ea))
//			}
//		}
//		list = append(list, bob)
//	}
//	return string(list)
//}

func getVMinfo(mask string) string {
	vminfo := []string{}
	cmdhost, err := minicli.CompileCommand(fmt.Sprintf(`vm info mask %s`, mask)) //local host stats
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	respHostChan := runCommand(cmdhost, false)

	g := <-respHostChan
	for _, row := range g[0].Tabular { //local host data
		tl := `<tr>`
		for _, entry := range row {
			tl += `<td>` + entry + `</td>`
		}
		tl += `</tr>`
		vminfo = append(vminfo, tl)
	}
	cmdhostall, err := minicli.CompileCommand(fmt.Sprintf(`mesh send all vm info mask %s`, mask)) //mesh send all host
	respHostAllChan := runCommand(cmdhostall, false)
	s := <-respHostAllChan
	if len(s) != 0 { //check if there are other hosts
		for _, row := range s[0].Tabular { //mesh data
			tl := `<tr>`
			for _, entry := range row {
				tl += `<td>` + entry + `</td>`
			}
			tl += `</tr>`
			vminfo = append(vminfo, tl)
		}
	}
	body := fmt.Sprintf(`<table border=1>%s</table>`, strings.Join(vminfo, "\n"))
	return fmt.Sprintf(body)
}
func guiStats() string {
	stats := []string{}
	cmdhost, err := minicli.CompileCommand("host") //local host stats
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	respHostChan := runCommand(cmdhost, false)

	g := <-respHostChan
	if len(stats) == 0 { //If stats is empty, i need a header
		header := `<tr>`
		for _, h := range g[0].Header {
			header += `<td>` + h + `</td>`
		}
		header += `</tr>`
		stats = append(stats, header)
	}

	for _, row := range g[0].Tabular { //local host data
		tl := `<tr>`
		for _, entry := range row {
			tl += `<td>` + entry + `</td>`
		}
		tl += `</tr>`
		stats = append(stats, tl)
	}
	cmdhostall, err := minicli.CompileCommand("mesh send all host") //mesh send all host
	respHostAllChan := runCommand(cmdhostall, false)
	s := <-respHostAllChan
	if len(s) != 0 { //check if there are other hosts
		for _, row := range s[0].Tabular { //mesh data
			tl := `<tr>`
			for _, entry := range row {
				tl += `<td>` + entry + `</td>`
			}
			tl += `</tr>`
			stats = append(stats, tl)
		}
	}
	body := fmt.Sprintf(`<table border=1>%s</table>`, strings.Join(stats, "\n"))
	return fmt.Sprintf(HTMLFRAME, body)
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
	//return fmt.Sprintf(`<html><title>Host List</title><body>%s</body></html>`, body.String())
	return fmt.Sprintf(HTMLFRAME, body.String())
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
	//return fmt.Sprintf(`<html><title>All VMs</title><body><table border=1>%s</table></body></html>`, strings.Join(info, "\n"))
	body := fmt.Sprintf(`<table border=1>%s</table>`, strings.Join(info, "\n"))
	return fmt.Sprintf(HTMLFRAME, body)
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

	//return fmt.Sprintf(`<html><title>VM list</title><body><table border=1>%s</table></body></html>`, strings.Join(lines, "\n"))
	body := fmt.Sprintf(`<table border=1>%s</table>`, strings.Join(lines, "\n"))
	return fmt.Sprintf(HTMLFRAME, body)
}
