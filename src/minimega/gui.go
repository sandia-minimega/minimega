// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
//Author: Brian Wright

package main

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
	//#WEB : Will need this line  when web.go is phased out
	newdefaultNoVNC string = "/opt/minimega/misc/novnc"
	newdefaultD3    string = "/opt/minimega/misc/d3"
	newdefaultTerm  string = "/opt/minimega/misc/terminal"
	HTMLFRAME              = `<!DOCTYPE html>
				<head><title>Minimega GUI</title>
				<link rel="stylesheet" type="text/css" href="/gui/d3/nav.css">
				<link rel="stylesheet" type="text/css" href="/gui/d3/jquery.dataTables.css">
				<script type="text/javascript" language="javascript" src="/gui/d3/jquery-1.11.1.min.js"></script>
				<script type="text/javascript" language="javascript" src="/gui/d3/jquery.dataTables.min.js"></script>
				%s
				</head>
				<body>
				<nav><ul>
				  <!--<li><a href="/gui/vnc">Host List</a></li>-->
				  <li><a href="/gui/all">All VMs</a></li>
				  <li><a href="/gui/stats">Host Stats</a></li>
				  <li><a href="/gui/errors">VM Errors</a></li>
				  <li><a href="/gui/state">State of Health</a></li>
				  <li><a href="/gui/map">VM Map</a></li>
				 <!-- <li><a href="/gui/graph">Graph</a></li>
				  <li><a href="/gui/terminal/terminal.html">Terminal(concept)</a></li>-->
				</ul></nav>      
				%s
				</body></html>`

	D3MAP = `    
		<div id="container"></div>
		<script src="/gui/d3/d3.min.js"></script>
		<script src="/gui/d3/topojson.v1.min.js"></script>
		<script>
		d3.select(window).on("resize", throttle);
		var zoom = d3.behavior.zoom().scaleExtent([1, 9]).on("zoom", move);
		var width = document.getElementById('container').offsetWidth;
		var height = width / 2;
		var topo,projection,path,svg,g;
		var graticule = d3.geo.graticule();
		var tooltip = d3.select("#container").append("div").attr("class", "tooltip hidden");
		setup(width,height);
		function setup(width,height){
		  projection = d3.geo.mercator().translate([(width/2), (height/2)]).scale( width / 2 / Math.PI);
		  path = d3.geo.path().projection(projection);
		  svg = d3.select("#container").append("svg")
		      .attr("width", width)
		      .attr("height", height)
		      .call(zoom)
		      .on("click", click)
		      .append("g");
		  g = svg.append("g").on("click", click);
		}
		d3.json("/gui/d3/world-topo-min.json", function(error, world) {
		  var countries = topojson.feature(world, world.objects.countries).features;
		  topo = countries;
		  draw(topo);
		});
		function draw(topo) {
		  svg.append("path")
		  svg.append("path").datum(graticule).attr("class", "graticule").attr("d", path);
		  g.append("path")
		   .datum({type: "LineString", coordinates: [[-180, 0], [-90, 0], [0, 0], [90, 0], [180, 0]]})
		   .attr("class", "equator")
		   .attr("d", path);
		  var country = g.selectAll(".country").data(topo);
		  country.enter().insert("path")
		      .attr("class", "country")
		      .attr("d", path)
		      .attr("id", function(d,i) { return d.id; })
		      .attr("title", function(d,i) { return d.properties.name; })
		      .style("fill", function(d, i) { return d.properties.color; });
		  //offsets for tooltips
		  var offsetL = document.getElementById('container').offsetLeft+20;
		  var offsetT = document.getElementById('container').offsetTop+10;
		  //tooltips
		  country.on("mousemove", function(d,i) {
		      var mouse = d3.mouse(svg.node()).map( function(d) { return parseInt(d); } );
		      tooltip.classed("hidden", false)
			     .attr("style", "left:"+(mouse[0]+offsetL)+"px;top:"+(mouse[1]+offsetT)+"px")
			     .html(d.properties.name);
		      })
		      .on("mouseout",  function(d,i) {
			tooltip.classed("hidden", true);
		      }); 
		%s
		}
		function redraw() {
		  width = document.getElementById('container').offsetWidth;
		  height = width / 2;
		  d3.select('svg').remove();
		  setup(width,height);
		  draw(topo);
		}
		function move() {
		  var t = d3.event.translate;
		  var s = d3.event.scale; 
		  zscale = s;
		  var h = height/4;
		  t[0] = Math.min(
		    (width/height)  * (s - 1), 
		    Math.max( width * (1 - s), t[0] )
		  );
		  t[1] = Math.min(
		    h * (s - 1) + h * s, 
		    Math.max(height  * (1 - s) - h * s, t[1])
		  );
		  zoom.translate(t);
		  g.attr("transform", "translate(" + t + ")scale(" + s + ")");
		  //adjust the country hover stroke width based on zoom level
		  d3.selectAll(".country").style("stroke-width", 1.5 / s);
		}
		var throttleTimer;
		function throttle() {
		  window.clearTimeout(throttleTimer);
		    throttleTimer = window.setTimeout(function() {
		      redraw();
		    }, 200);
		}
		//geo translation on mouse click in map
		function click() {
		  var latlon = projection.invert(d3.mouse(this));
		  console.log(latlon);
		}
		//function to add points and text to the map (used in plotting capitals)
		function addpoint(lat,lon,text) {
		  var gpoint = g.append("g").attr("class", "gpoint").attr("xlink:href","http://www.sandia.gov");
		  var x = projection([lat,lon])[0];
		  var y = projection([lat,lon])[1];
		  gpoint.append("svg:circle").attr("cx", x).attr("cy", y).attr("class","point").attr("r", 1.5);
		  if(text.length>0){    //conditional in case a point has no associated text
		    gpoint.append("text").attr("x", x+2).attr("y", y+2).attr("class","text").text(text);
		  }
		}
		</script>
		`
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
		resp.Error = "GUI is already running"
	} else {
		go guiStart(port, noVNC, d3, term)
	}

	return resp
}

func guiStart(port, noVNC string, d3 string, term string) {
	guiRunning = true
	http.Handle("/gui/novnc/", http.StripPrefix("/gui/novnc/", http.FileServer(http.Dir(noVNC))))
	http.Handle("/gui/terminal/", http.StripPrefix("/gui/terminal/", http.FileServer(http.Dir(term))))
	http.Handle("/gui/d3/", http.StripPrefix("/gui/d3/", http.FileServer(http.Dir(d3))))
	http.Handle("/gui/graph/", http.StripPrefix("/gui/graph/", http.FileServer(http.Dir("/opt/minimega/misc/d3/force"))))

	http.HandleFunc("/gui/ws/", vncWsHandler)
	http.HandleFunc("/gui/map", guiMapVMs)
	http.HandleFunc("/gui/errors", guiErrorVMs)
	http.HandleFunc("/gui/state", guiState)
	http.HandleFunc("/gui/stats", guiStats)
	http.HandleFunc("/gui/all", guiAllVMs)
	http.HandleFunc("/gui/", guiRoot)
	http.HandleFunc("/gui/command/", guiCmd)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Error("guiStart: %v", err)
		guiRunning = false
	}
}

func guiCmd(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimSpace(r.URL.String())
	fields := strings.Split(url, "/")
	cmd := fields[3]

	if cmd == "start" {
		mmstartcmd, err := minicli.CompileCommand(fmt.Sprintf(`mesh send all vm start all`))
		if err != nil {
			log.Fatalln(err)
		}
		runCommand(mmstartcmd, true)
		mmstartLcmd, err := minicli.CompileCommand(fmt.Sprintf(`vm start all`))
		if err != nil {
			log.Fatalln(err)
		}
		runCommand(mmstartLcmd, true)
	}
	if cmd == "flush" {
		mmflushcmd, err := minicli.CompileCommand(fmt.Sprintf(`mesh send all vm flush`))
		if err != nil {
			log.Fatalln(err)
		}
		runCommand(mmflushcmd, true)
		mmflushLcmd, err := minicli.CompileCommand(fmt.Sprintf(`vm flush`))
		if err != nil {
			log.Fatalln(err)
		}
		runCommand(mmflushLcmd, true)
	}
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
	case 1: // "/gui"
		w.Write([]byte(guiHome()))
	case 2: // "/gui/vnc/"
		w.Write([]byte(guiHosts()))
	case 3: // "/gui/vnc/<host>/"
		w.Write([]byte(guiHostVMs(fields[2])))
	case 4: // "/gui/vnc/<host>/<port>"
		title := html.EscapeString(fields[2] + ":" + fields[3]) //change to vm NAME
		path := fmt.Sprintf("/gui/novnc/vnc_auto.html?title=%v&path=gui/ws/%v/%v", title, fields[2], fields[3])
		http.Redirect(w, r, path, http.StatusTemporaryRedirect)
	default:
		http.NotFound(w, r)
	}
}

func guiHome() string {
	return fmt.Sprintf(HTMLFRAME, "", "")
}

func guiState(w http.ResponseWriter, r *http.Request) {

	mask := `id,name,tags`
	list := getVMinfo(mask)
	vdata := ``
	for _, row := range list {
		if len(row) != 3 {
			log.Fatal("column count mismatch: %v", row)
		}
		id := row[0]
		name := row[1]

		var trt string
		var ping string
		f := strings.Fields(row[2])
		for _, v := range f {
			v = strings.Trim(v, "[]")
			v2 := strings.Split(v, ":")
			if len(v2) != 2 {
				continue
			}
			if strings.Contains(v2[0], "trt") {
				trt = v2[1]
			} else if strings.Contains(v2[0], "ping") {
				ping = v2[1]
			}
		}
		if trt == "" || ping == "" {
			continue
		}
		vdata += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`, name, id, trt, ping)
	}
	header := `<thead><tr><th>name</th><th>id</th><th>trt</th><th>ping</th></thead>`
	tabletype := `<script type="text/javascript" language="javascript" src="/gui/d3/table.js"></script>`
	body := fmt.Sprintf(`<table id="example" class="hover" cellspacing="0"> %s <tbody> %s </tbody></table>`, header, vdata)
	w.Write([]byte(fmt.Sprintf(HTMLFRAME, tabletype, body)))
}

func guiMapVMs(w http.ResponseWriter, r *http.Request) {
	mask := `id,name,tags`
	list := getVMinfo(mask)
	dataformat := `   addpoint(%s,%s,"%s")` + "\n"
	mapdata := ``
	for _, row := range list {
		if len(row) != 3 {
			log.Fatal("column count mismatch: %v", row)
		}
		//id := row[0]
		name := row[1]

		// grab out lat/long
		var lat string
		var long string
		f := strings.Fields(row[2])
		for _, v := range f {
			v = strings.Trim(v, "[]")
			v2 := strings.Split(v, ":")
			if len(v2) != 2 {
				continue
			}
			if strings.Contains(v2[0], "lat") {
				lat = v2[1]
			} else if strings.Contains(v2[0], "long") {
				long = v2[1]
			}
		}
		if lat == "" || long == "" {
			continue
		}
		mapdata += fmt.Sprintf(dataformat, lat, long, name)
	}

	d3body := fmt.Sprintf(D3MAP, mapdata)
	d3head := `<link rel="stylesheet" type="text/css" href="/gui/d3/d3map.css">`
	w.Write([]byte(fmt.Sprintf(HTMLFRAME, d3head, d3body)))
}

func getVMinfo(mask string) [][]string {
	var tabular [][]string

	cmdHost, err := minicli.CompileCommand(fmt.Sprintf(`.columns %s vm info`, mask))
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	respChan := runCommand(cmdHost, false)

	for r := range respChan {
		tabular = append(tabular, r[0].Tabular...)
	}

	cmdHostAll, err := minicli.CompileCommand(fmt.Sprintf(`.columns %s mesh send all vm info`, mask))
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}
	respChan = runCommand(cmdHostAll, false)

	for r := range respChan {
		for _, resp := range r {
			if len(r) != 0 {
				tabular = append(tabular, resp.Tabular...)
			}
		}
	}

	return tabular
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
	//<a class="toggle-vis" data-column="5">Salary</a>
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
	//s := <-respHostAllChan
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
	//body := fmt.Sprintf(`<table border=1>%s</table>`, strings.Join(stats, "\n"))
	body := fmt.Sprintf(`<table id="example" class="hover" cellspacing="0"> %s </tbody></table>`, strings.Join(stats, "\n"))

	//w.Write([]byte(fmt.Sprintf(HTMLFRAME, body)))
	tabletype := `<script type="text/javascript" language="javascript" src="/gui/d3/stats.js"></script>`
	w.Write([]byte(fmt.Sprintf(HTMLFRAME, tabletype, body)))
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

	cmd, err := minicli.CompileCommand(".columns id,state mesh send all vm info")
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}

	remoteRespChan := runCommand(cmd, false)

	//Calculate total VMs in experiment
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
	tabletype := `<script type="text/javascript" language="javascript" src="/gui/d3/table.js"></script>`
	return fmt.Sprintf(HTMLFRAME, tabletype, body.String())
}

func guiErrorVMs(writer http.ResponseWriter, request *http.Request) {
	var resp chan minicli.Responses
	var respAll chan minicli.Responses
	mask := "id,name,state,memory,vcpus,disk,initrd,kernel,cdrom,mac,bridge,ip,vlan,append,tags"
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
			//This is an ABSURD way to get the local host name:
			var HostChan chan minicli.Responses
			bob, _ := minicli.CompileCommand(fmt.Sprintf("host name"))
			HostChan = runCommand(bob, false)
			H := <-HostChan
			Host := H[0].Response

			format := `<tr><td>%v</td><td>%v</td><td><a href="/gui/vnc/%v/%v">%v</a></td>`
			tl := fmt.Sprintf(format, id, Host, Host, 5900+id, r[1])
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

func guiAllVMs(writer http.ResponseWriter, request *http.Request) {
	var resp chan minicli.Responses
	var respAll chan minicli.Responses
	mask := "id,name,state,memory,vcpus,disk,initrd,kernel,cdrom,mac,bridge,ip,vlan,append,tags"
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
		if r[2] != "ERROR" && r[2] != "QUIT" {
			id, err := strconv.Atoi(r[0])
			if err != nil {
				log.Errorln(err)
				return
			}
			//This is an ABSURD way to get the local host name:
			var HostChan chan minicli.Responses
			bob, _ := minicli.CompileCommand(fmt.Sprintf("host name"))
			HostChan = runCommand(bob, false)
			H := <-HostChan
			Host := H[0].Response

			format := `<tr><td>%v</td><td>%v</td><td><a href="/gui/vnc/%v/%v">%v</a></td>`
			tl := fmt.Sprintf(format, id, Host, Host, 5900+id, r[1])
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
	body := fmt.Sprintf(`<table id="example" class="hover" cellspacing="0"> %s </tbody></table>`, strings.Join(info, "\n"))
	tabletype := `<script type="text/javascript" language="javascript" src="/gui/d3/table.js"></script>`
	writer.Write([]byte(fmt.Sprintf(HTMLFRAME, tabletype, body)))
}

func guiHostVMs(host string) string {
	var respChan chan minicli.Responses

	mask := "id,name,state,memory,vcpus,disk,initrd,kernel,cdrom,mac,bridge,ip,vlan,append,tags"
	cmdLocal, err := minicli.CompileCommand(fmt.Sprintf(".columns %v vm info", mask))
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}

	cmdRemote, err := minicli.CompileCommand(fmt.Sprintf(".columns %v mesh send %v vm info", mask, host))
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
				header := `<thead><tr>`
				for _, h := range resp.Header {
					header += `<th>` + h + `</th>`
				}
				header += `</tr></thead>`
				lines = append(lines, header)
			}

			for _, row := range resp.Tabular {
				if row[2] != "ERROR" && row[2] != "QUIT" {
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

	body := fmt.Sprintf(`<table id="example" class="hover" cellspacing="0"> %s </tbody></table>`, strings.Join(lines, "\n"))
	tabletype := `<script type="text/javascript" language="javascript" src="/gui/d3/table.js"></script>`
	return fmt.Sprintf(HTMLFRAME, tabletype, body)
}
