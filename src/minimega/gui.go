// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
//Author: Brian Wright

package main

import (
	"fmt"
	"html"
	"html/template"
	"minicli"
	log "minilog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	GUI_PORT       = 9001
	defaultWebroot = "misc/web"
	friendlyError  = "oops, something went wrong"
)

type htmlTable struct {
	Header  []string
	Tabular [][]string
	ID      string
	Class   string
}

var (
	guiRunning bool
	webroot    string = defaultWebroot
	server     *http.Server
	HTMLFRAME  string
	D3MAP      string

	guiTemplates *template.Template
)

var guiCLIHandlers = []minicli.Handler{
	{ // gui
		HelpShort: "start the minimega GUI",
		HelpLong: `
Launch the GUI webserver

The webserver requires noVNC and D3, expecting to find them in
subdirectories "novnc" and "d3" under misc/web/ by default. To set a
different path, run:

        gui webroot <path to web dir>

Once you have set the path, or if the default is acceptable, run "gui" to
start the web server on the default port 9001:

        gui

To start the webserver on a specific port, issue the web command with the port:

	gui 9001

NOTE: If you start the GUI with an invalid webroot, you can safely
re-run "gui webroot" followed by "gui" to update it.

`,
		Patterns: []string{
			"gui [port]",
			"gui webroot <path>",
		},
		Call: wrapSimpleCLI(cliGUI),
	},
}

func init() {
	registerHandlers("gui", guiCLIHandlers)

}

func cliGUI(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	port := GUI_PORT
	if c.StringArgs["port"] != "" {
		// Check if port is an integer
		p, err := strconv.Atoi(c.StringArgs["port"])
		if err != nil {
			resp.Error = fmt.Sprintf("'%v' is not a valid port", c.StringArgs["port"])
			return resp
		}

		port = p
	}

	if c.StringArgs["path"] != "" {
		webroot = c.StringArgs["path"]
		return resp
	}

	go guiStart(port)

	return resp
}

func guiStart(port int) {
	guiRunning = true

	// re-initialize templates
	var err error
	guiTemplates, err = template.ParseGlob(filepath.Join(webroot, "templates", "*.html"))
	if err != nil {
		log.Error("guiStart: couldn't initalize templates: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/novnc/", http.StripPrefix("/novnc/", http.FileServer(http.Dir(filepath.Join(webroot, "novnc")))))
	mux.Handle("/d3/", http.StripPrefix("/d3/", http.FileServer(http.Dir(filepath.Join(webroot, "d3")))))

	mux.HandleFunc("/ws/", vncWsHandler)
	mux.HandleFunc("/map", guiMapVMs)
	mux.HandleFunc("/errors", guiErrorVMs)
	mux.HandleFunc("/state", guiState)
	mux.HandleFunc("/stats", guiStats)
	mux.HandleFunc("/all", guiAllVMs)
	mux.HandleFunc("/tile", guiTiler)
	mux.HandleFunc("/vnc/", guiVNC)
	mux.HandleFunc("/command/", guiCmd)
	mux.HandleFunc("/screenshot/", guiScreenshot)
	mux.HandleFunc("/", guiHome)

	if server == nil {
		server = &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		}

		err := server.ListenAndServe()
		if err != nil {
			log.Error("guiStart: %v", err)
			guiRunning = false
		}
	} else {
		// just update the mux
		server.Handler = mux
	}
}

func guiRenderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	if err := guiTemplates.ExecuteTemplate(w, tmpl, data); err != nil {
		log.Error("unable to execute template %s -- %v", tmpl, err)
		http.Error(w, friendlyError, http.StatusInternalServerError)
	}
}

func guiScreenshot(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimSpace(r.URL.String())
	urlFields := strings.Split(url, "/")

	if len(urlFields) != 4 {
		w.Write([]byte("usage: screenshot/<hostname>_<vm id>.png<br>usage: screenshot<hostname>_<vm id>_<max size>.png"))
		return
	}

	fields := strings.Split(urlFields[3], "_")
	if len(fields) != 2 && len(fields) != 3 {
		w.Write([]byte("usage: screenshot/<hostname>_<vm id>.png<br>usage: screenshot<hostname>_<vm id>_<max size>.png"))
		return
	}

	host := fields[0]
	var vmId string
	var max string
	if len(fields) == 2 {
		vmId = strings.TrimSuffix(fields[1], ".png")
	} else {
		vmId = fields[1]
		max = strings.TrimSuffix(fields[2], ".png")
	}

	var respChan chan minicli.Responses

	cmdLocal, err := minicli.CompileCommand(fmt.Sprintf("vm screenshot %v %v", vmId, max))
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}

	cmdRemote, err := minicli.CompileCommand(fmt.Sprintf("mesh send %v vm screenshot %v %v", host, vmId, max))
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}

	if host == hostname {
		respChan = runCommand(cmdLocal, false)
	} else {
		respChan = runCommand(cmdRemote, false)
	}

	for resps := range respChan {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				w.Write([]byte(resp.Error))
				continue
			}

			if resp.Data == nil {
				w.Write([]byte("no png data!"))
				continue
			}

			d := resp.Data.([]byte)
			w.Write(d)
		}
	}
}

func guiCmd(w http.ResponseWriter, r *http.Request) {
	host := ""
	cmd := ""
	query := r.URL.Query()
	cmd = query.Get("cmd")
	host = query.Get("host")
	fmt.Println(cmd)
	fmt.Println(host)
	mm, err := DialMinimega()
	if err != nil {
		log.Fatalln(err)
	}

	if cmd != "" {
		fmt.Println("Cmd:", cmd)
		cmd, err2 := minicli.CompileCommand(cmd)
		if err2 != nil {
			log.Error("%v", err2)
			return
		}
		body := ""
		if cmd != nil {
			for resp := range mm.runCommand(cmd) {
				body = strings.Replace(resp.Rendered, "\n", "<br>", -1)
			}
		}
		w.Write([]byte(body))
		//fmt.Sprintf(HTMLFRAME, "", body)))
	} else {
		fmt.Println("ERROR: No command given.")
	}

	//url := strings.TrimSpace(r.URL.String())
	//fields := strings.Split(url, "/")
	//cmd := fields[3]

	//if cmd == "start" {
	//	mmstartcmd, err := minicli.CompileCommand(fmt.Sprintf(`mesh send all vm start all`))
	//	if err != nil {
	//		log.Fatalln(err)
	//	}
	//	localstartrespchan := runCommand(mmstartcmd, true)
	//	for range localstartrespchan {
	//	}
	//	mmstartLcmd, err := minicli.CompileCommand(fmt.Sprintf(`vm start all`))
	//	if err != nil {
	//		log.Fatalln(err)
	//	}
	//	allstartrespchan := runCommand(mmstartLcmd, true)
	//	for range allstartrespchan {
	//	}
	//}
	//if cmd == "flush" {
	//	mmflushcmd, err := minicli.CompileCommand(fmt.Sprintf(`mesh send all vm flush`))
	//	if err != nil {
	//		log.Fatalln(err)
	//	}
	//	localflushrespchan := runCommand(mmflushcmd, true)
	//	for range localflushrespchan {
	//	}
	//	mmflushLcmd, err := minicli.CompileCommand(fmt.Sprintf(`vm flush`))
	//	if err != nil {
	//		log.Fatalln(err)
	//	}
	//	allflushrespchan := runCommand(mmflushLcmd, true)
	//	for range allflushrespchan {
	//	}
	//}
}

func guiVNC(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimSpace(r.URL.String())
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	fields := strings.Split(url, "/")
	fields = fields[1 : len(fields)-1]
	if len(fields) == 4 {
		title := html.EscapeString(fields[2] + ":" + fields[3]) //change to vm NAME
		path := fmt.Sprintf("/gui/novnc/vnc_auto.html?title=%v&path=gui/ws/%v/%v", title, fields[2], fields[3])
		iframeresize := `<script>
                         	var buffer = 20; //scroll bar buffer
			 	var iframe = document.getElementById('vnc');

			 	function pageY(elem) {
    					return elem.offsetParent ? (elem.offsetTop + pageY(elem.offsetParent)) : elem.offsetTop;
				}

				function resizeIframe() {
    					var height = document.documentElement.clientHeight;
    					height -= pageY(document.getElementById('vnc'))+ buffer ;
   					height = (height < 0) ? 0 : height;
    					document.getElementById('vnc').style.height = height + 'px';
				}

				window.onresize = resizeIframe;
				window.onload = resizeIframe;
         		   </script>
			  `

		body := fmt.Sprintf(`<iframe id="vnc" width="100%v" src="%v"></iframe>`, "%", path)
		w.Write([]byte(fmt.Sprintf(HTMLFRAME, iframeresize, body)))
	} else {
		http.NotFound(w, r)
	}
}

func guiHome(w http.ResponseWriter, r *http.Request) {
	guiRenderTemplate(w, "home.html", nil)
}

func guiState(w http.ResponseWriter, r *http.Request) {
	table := htmlTable{
		Header:  []string{"name", "id", "traceroute", "SNMP", "DNS", "app"},
		Tabular: [][]string{},
		ID:      "example",
		Class:   "hover",
	}

OuterLoop:
	for _, row := range globalVmInfo("id,name,tags") {
		if len(row) != 3 {
			log.Fatal("column count mismatch: %v", row)
		}

		tags, err := ParseVmTags(row[2])
		if err != nil {
			log.Error("unable to parse vm tags (%s) -- %v", strings.Join(row[:2], ":"), err)
		}

		// Start with ID, name
		row = row[:2]

		// TODO: Are these the right keys?
		for _, tag := range []string{"traceroute", "SNMP", "DNS", "app"} {
			if tags[tag] == "" {
				log.Debug("skipping vm (%s) -- missing tag %s", strings.Join(row[:2], ":"), tag)
				continue OuterLoop
			}

			row = append(row, tags[tag])
		}

		table.Tabular = append(table.Tabular, row)
	}

	guiRenderTemplate(w, "state.html", table)
}

func guiMapVMs(w http.ResponseWriter, r *http.Request) {
	type point struct {
		Lat, Long float64
		Text      string
	}

	points := []point{}

	for _, row := range globalVmInfo("id,name,tags") {
		if len(row) != 3 {
			log.Fatal("column count mismatch: %v", row)
		}

		name := strings.Join(row[:2], ":")

		tags, err := ParseVmTags(row[2])
		if err != nil {
			log.Error("unable to parse vm tags for %s -- %v", name, err)
		}

		p := point{Text: name}

		// TODO: Are these the right keys?
		if tags["lat"] == "" || tags["long"] == "" {
			log.Debug("skipping vm %s -- missing required tags lat/long", name)
			continue
		}

		p.Lat, err = strconv.ParseFloat(tags["lat"], 64)
		if err != nil {
			log.Error("invalid lat for vm %s -- expected float")
			continue
		}

		p.Long, err = strconv.ParseFloat(tags["lat"], 64)
		if err != nil {
			log.Error("invalid lat for vm %s -- expected float")
			continue
		}

		points = append(points, p)
	}

	guiRenderTemplate(w, "map.html", points)
}

func guiStats(w http.ResponseWriter, r *http.Request) {
	stats := []string{}
	cmdhost, err := minicli.CompileCommand("host") //local host stats
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	respHostChan := runCommand(cmdhost, false)
	g := <-respHostChan
	if len(stats) == 0 { //If stats is empty, i need a header
		header := `<thead><tr>`
		for _, h := range g[0].Header {
			header += `<th>` + h + `</th>`
		}
		header += `</tr></thead><tbody>`
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
	for s := range respHostAllChan {
		if len(s) != 0 { //check if there are other hosts
			for _, node := range s {
				for _, row := range node.Tabular { //mesh data
					tl := `<tr>`
					for _, entry := range row {
						tl += `<td>` + entry + `</td>`
					}
					tl += `</tr>`
					stats = append(stats, tl)
				}
			}
		}
	}
	body := fmt.Sprintf(`<table id="example" class="hover" cellspacing="0"> %s </tbody></table>`, strings.Join(stats, "\n"))
	tabletype := `<script type="text/javascript" language="javascript" src="/gui/d3/stats.js"></script>`
	w.Write([]byte(fmt.Sprintf(HTMLFRAME, tabletype, body)))
}

func guiErrorVMs(writer http.ResponseWriter, request *http.Request) {
	var resp chan minicli.Responses
	var respAll chan minicli.Responses
	mask := "id,name,state,memory,vcpus,migrate,disk,snapshot,initrd,kernel,cdrom,append,bridge,tap,mac,ip,ip6,vlan,uuid,cc_active,tags"
	cmdLocal, err := minicli.CompileCommand(".columns " + mask + " vm info")
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	cmdRemote, err := minicli.CompileCommand(fmt.Sprintf(".columns %s mesh send all vm info", mask))
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
		header := `<thead><tr>`
		for _, h := range ga {
			header += `<th>` + h + `</th>`
			if h == "id" {
				header += `<th>` + `host` + `</th>`
			}
		}
		header += `</tr></thead><tbody>`
		info = append(info, header)
	}

	r := g[0].Tabular
	for _, r := range r {
		if r[2] == "ERROR" {
			id, err := strconv.Atoi(r[0])
			if err != nil {
				log.Errorln(err)
				return
			}

			format := `<tr><td>%v</td><td>%v</td><td><a href="/gui/vnc/%v/%v">%v</a></td>`
			tl := fmt.Sprintf(format, id, hostname, hostname, 5900+id, r[1])
			for _, entry := range r[2:] {
				tl += `<td>` + entry + `</td>`
			}
			tl += `</tr>`
			info = append(info, tl)
		}
	}
	//get mesh response
	for sa := range respAll {
		if len(sa) != 0 {
			for _, node := range sa {
				for _, s := range node.Tabular {
					if s[2] == "ERROR" {
						id, err := strconv.Atoi(s[0])
						if err != nil {
							log.Errorln(err)
							return
						}

						format := `<tr><td>%v</td><td>%v</td><td><a href="/gui/vnc/%v/%v">%v</a></td>`
						tl := fmt.Sprintf(format, id, node.Host, node.Host, 5900+id, s[1])
						for _, entry := range s[2:] {
							tl += `<td>` + entry + `</td>`
						}
						tl += `</tr>`
						info = append(info, tl)
					}
				}
			}
		}
	}
	body := fmt.Sprintf(`<table id="example" class="hover" cellspacing="0"> %s </tbody></table>`, strings.Join(info, "\n"), `<br>insert flush button here<br>insert start button here`)
	tabletype := `<script type="text/javascript" language="javascript" src="/gui/d3/table.js"></script>`
	writer.Write([]byte(fmt.Sprintf(HTMLFRAME, tabletype, body)))
}

func guiTiler(writer http.ResponseWriter, request *http.Request) {
	var resp chan minicli.Responses
	var respAll chan minicli.Responses
	mask := "id,name,state"
	cmdLocal, err := minicli.CompileCommand(".columns " + mask + " vm info")
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	cmdRemote, err := minicli.CompileCommand(fmt.Sprintf(".columns %s mesh send all vm info", mask))
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	resp = runCommand(cmdLocal, false)
	respAll = runCommand(cmdRemote, false)

	format := `<div style="float: left; position: relative; padding-right: 4px; padding-bottom: 3px;"><a href="/gui/vnc/%v/%v"><img src="/gui/screenshot/%v_%v_250.png" alt="%v" /></a></div>`
	info := []string{}
	g := <-resp
	r := g[0].Tabular
	for _, r := range r {
		if r[2] != "ERROR" && r[2] != "QUIT" {
			id, err := strconv.Atoi(r[0])
			if err != nil {
				log.Errorln(err)
				return
			}
			tl := fmt.Sprintf(format, hostname, 5900+id, hostname, id, r[1])
			info = append(info, tl)
		}
	}
	//get mesh response
	for sa := range respAll {
		if len(sa) != 0 {
			for _, node := range sa {
				for _, s := range node.Tabular {
					if s[2] != "ERROR" && s[2] != "QUIT" {
						id, err := strconv.Atoi(s[0])
						if err != nil {
							log.Errorln(err)
							return
						}

						tl := fmt.Sprintf(format, node.Host, 5900+id, node.Host, id, s[1])
						info = append(info, tl)
					}
				}
			}
		}
	}
	body := fmt.Sprintf(`<div style="overflow: hidden; margin: 10px;" > %s </div>`, strings.Join(info, "\n"))
	writer.Write([]byte(fmt.Sprintf(HTMLFRAME, "", body)))
}

func guiAllVMs(writer http.ResponseWriter, request *http.Request) {
	var resp chan minicli.Responses
	var respAll chan minicli.Responses
	columnnames := []string{}
	mask := "id,name,state,memory,vcpus,migrate,disk,snapshot,initrd,kernel,cdrom,append,bridge,tap,mac,ip,ip6,vlan,uuid,cc_active,tags"
	format := `<tr><td><a href="/gui/vnc/%v/%v"><img src="/gui/screenshot/%v_%v_140.png" alt="%v" /></a></td><td>%v</td><td>%v</td><td><a href="/gui/vnc/%v/%v">%v</a></td>`
	cmdLocal, err := minicli.CompileCommand(".columns " + mask + " vm info")
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	cmdRemote, err := minicli.CompileCommand(fmt.Sprintf(".columns %s mesh send all vm info", mask))
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
		header := `<thead><tr><th>snapshot</th>`
		columnnames = append(columnnames, "snapshot")
		for _, h := range ga {
			header += `<th>` + h + `</th>`
			columnnames = append(columnnames, h)
			if h == "id" {
				header += `<th>` + `host` + `</th>`
				columnnames = append(columnnames, "host")
			}
		}
		header += `</tr></thead><tbody>`
		info = append(info, header)
	}

	bob := g[0].Tabular
	for _, r := range bob {
		if r[2] != "ERROR" && r[2] != "QUIT" {
			id, err := strconv.Atoi(r[0])
			if err != nil {
				log.Errorln(err)
				return
			}

			tl := fmt.Sprintf(format, hostname, 5900+id, hostname, id, r[1], id, hostname, hostname, 5900+id, r[1])
			for _, entry := range r[2:] {
				tl += `<td>` + entry + `</td>`
			}
			tl += `</tr>`
			info = append(info, tl)
		}
	}
	//get mesh response
	for sa := range respAll {
		if len(sa) != 0 {
			for _, node := range sa {
				for _, s := range node.Tabular {
					if s[2] != "ERROR" && s[2] != "QUIT" {
						id, err := strconv.Atoi(s[0])
						if err != nil {
							log.Errorln(err)
							return
						}
						tl := fmt.Sprintf(format, node.Host, 5900+id, node.Host, id, s[1], id, node.Host, node.Host, 5900+id, s[1])
						for _, entry := range s[2:] {
							tl += `<td>` + entry + `</td>`
						}
						tl += `</tr>`
						info = append(info, tl)
					}
				}
			}
		}
	}
	columnviz := `<div style="color:#006400"> Toggle Columns: `
	for i, column := range columnnames {
		columnviz = columnviz + fmt.Sprintf(`<a class="toggle-vis" data-column="%v">%v</a>`, i, column)
		if i != len(columnnames) {
			columnviz = columnviz + " | "
		}
	}
	columnviz = columnviz + "</div>"
	body := fmt.Sprintf(`<table id="example" class="hover" cellspacing="0"> %s </tbody></table>`, strings.Join(info, "\n")) + columnviz
	tabletype := `<script type="text/javascript" language="javascript" src="/gui/d3/table.js"></script>`
	writer.Write([]byte(fmt.Sprintf(HTMLFRAME, tabletype, body)))
}
