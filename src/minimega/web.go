// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
//Author: Brian Wright

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"minicli"
	log "minilog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultWebPort = 9001
	defaultWebRoot = "misc/web"
	friendlyError  = "oops, something went wrong"
)

type htmlTable struct {
	Header  []string
	Tabular [][]interface{}
	ID      string
	Class   string
}

type vmScreenshotParams struct {
	Host string
	Name string
	Port int
	ID   int
	Size int
}

var web struct {
	Running   bool
	Server    *http.Server
	Templates *template.Template
}

var webCLIHandlers = []minicli.Handler{
	{ // web
		HelpShort: "start the minimega webserver",
		HelpLong: `
Launch the minimega webserver

The webserver requires noVNC and D3, expecting to find them in subdirectories
"novnc" and "d3" under misc/web/ by default. To set a different path, run:

        web webroot <path to web dir>

Once you have set the path, or if the default is acceptable, run "web" to start
the web server on the default port 9001:

        web

To start the webserver on a specific port, issue the web command with the port:

	web 9001

NOTE: If you start the webserver with an invalid webroot, you can safely re-run
"web root" followed by "web" to update it. You cannot, however, change the
server's port.`,
		Patterns: []string{
			"web [port]",
			"web root <path> [port]",
		},
		Call: wrapSimpleCLI(cliWeb),
	},
}

func init() {
	registerHandlers("web", webCLIHandlers)

}

func cliWeb(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	port := defaultWebPort
	if c.StringArgs["port"] != "" {
		// Check if port is an integer
		p, err := strconv.Atoi(c.StringArgs["port"])
		if err != nil {
			resp.Error = fmt.Sprintf("'%v' is not a valid port", c.StringArgs["port"])
			return resp
		}

		port = p
	}

	root := defaultWebRoot
	if c.StringArgs["path"] != "" {
		root = c.StringArgs["path"]
	}

	go webStart(port, root)

	return resp
}

func webStart(port int, root string) {
	// Initialize templates
	var err error
	web.Templates, err = template.ParseGlob(filepath.Join(root, "templates", "*.html"))
	if err != nil {
		log.Error("webStart: couldn't initalize templates: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/novnc/", http.StripPrefix("/novnc/", http.FileServer(http.Dir(filepath.Join(root, "novnc")))))
	mux.Handle("/d3/", http.StripPrefix("/d3/", http.FileServer(http.Dir(filepath.Join(root, "d3")))))

	mux.HandleFunc("/", webHome)
	mux.HandleFunc("/vms", webVMs)
	mux.HandleFunc("/map", webMapVMs)
	mux.HandleFunc("/screenshot/", webScreenshot)
	mux.HandleFunc("/stats", webStats)
	mux.HandleFunc("/tiles", webTileVMs)
	mux.HandleFunc("/vnc/", webVNC)
	mux.HandleFunc("/ws/", vncWsHandler)

	if web.Server == nil {
		web.Server = &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		}

		err := web.Server.ListenAndServe()
		if err != nil {
			log.Error("webStart: %v", err)
		} else {
			web.Running = true
		}
	} else {
		// just update the mux
		web.Server.Handler = mux
	}
}

// webRenderTemplate renders the given template with the provided data, writing
// the result to the client. Should be called last in an http handler.
func webRenderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	if err := web.Templates.ExecuteTemplate(w, tmpl, data); err != nil {
		log.Error("unable to execute template %s -- %v", tmpl, err)
		http.Error(w, friendlyError, http.StatusInternalServerError)
	}
}

// webScreenshot serves routes like /screenshot/<host>/<id>.png. Optional size
// query parameter dictates the size of the screenshot.
func webScreenshot(w http.ResponseWriter, r *http.Request) {
	fields := strings.Split(r.URL.Path, "/")
	if len(fields) != 4 {
		http.NotFound(w, r)
		return
	}
	fields = fields[2:]

	size := r.URL.Query().Get("size")
	host := fields[0]
	id := strings.TrimSuffix(fields[1], ".png")

	cmdStr := fmt.Sprintf("vm screenshot %s %s", id, size)
	if host != hostname {
		cmdStr = fmt.Sprintf("mesh send %s %s", host, cmdStr)
	}

	cmd := minicli.MustCompile(cmdStr)

	var screenshot []byte

	for resps := range runCommand(cmd, false) {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				http.Error(w, friendlyError, http.StatusInternalServerError)
				continue
			}

			if resp.Data == nil {
				http.NotFound(w, r)
			}

			if screenshot == nil {
				screenshot = resp.Data.([]byte)
			} else {
				log.Error("received more than one response for vm screenshot")
			}
		}
	}

	if screenshot != nil {
		w.Write(screenshot)
	} else {
		http.NotFound(w, r)
	}
}

// webVNC serves routes like /vnc/<host>/<port>/<vmName>.
func webVNC(w http.ResponseWriter, r *http.Request) {
	fields := strings.Split(r.URL.Path, "/")
	if len(fields) != 5 {
		http.NotFound(w, r)
		return
	}
	fields = fields[2:]

	host := fields[0]
	port := fields[1]
	vm := fields[2]

	u, _ := url.Parse("/novnc/vnc_auto.html")
	q := u.Query()
	q.Set("title", fmt.Sprintf("%s:%s", host, vm))
	q.Set("path", fmt.Sprintf("ws/%s/%s", host, port))
	u.RawQuery = q.Encode()

	webRenderTemplate(w, "vnc.html", u.String())
}

func webHome(w http.ResponseWriter, r *http.Request) {
	webRenderTemplate(w, "home.html", nil)
}

func webMapVMs(w http.ResponseWriter, r *http.Request) {
	type point struct {
		Lat, Long float64
		Text      string
	}

	masks := []string{"id", "name", "tags"}

	points := []point{}

	for _, rows := range globalVmInfo(masks, nil) {
		for _, row := range rows {
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
	}

	webRenderTemplate(w, "map.html", points)
}

func webStats(w http.ResponseWriter, r *http.Request) {
	table := htmlTable{
		Header:  []string{},
		Tabular: [][]interface{}{},
		ID:      "example",
		Class:   "hover",
	}

	cmd, err := minicli.CompileCommand("host")
	if err != nil {
		// Should never happen
		log.Fatalln(err)
	}

	for resps := range runCommandGlobally(cmd, false) {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			if len(table.Header) == 0 && len(resp.Header) > 0 {
				table.Header = append(table.Header, resp.Header...)
			}

			for _, row := range resp.Tabular {
				res := []interface{}{}
				for _, v := range row {
					res = append(res, v)
				}
				table.Tabular = append(table.Tabular, res)
			}
		}
	}

	webRenderTemplate(w, "stats.html", table)
}

func webVMs(w http.ResponseWriter, r *http.Request) {
	table := htmlTable{
		Header:  []string{"host", "screenshot"},
		Tabular: [][]interface{}{},
		ID:      "example",
		Class:   "hover",
	}
	table.Header = append(table.Header, vmMasks...)

	idIdx := mustFindMask("id")
	stateIdx := mustFindMask("state")
	nameIdx := mustFindMask("name")

	for host, rows := range globalVmInfo(vmMasks, nil) {
		for _, row := range rows {
			id, err := strconv.Atoi(row[idIdx])
			if err != nil {
				log.Errorln(err)
				continue
			}

			var buf bytes.Buffer
			if row[stateIdx] != "QUIT" && row[stateIdx] != "ERROR" {
				vm := vmScreenshotParams{
					Host: host,
					Name: row[nameIdx],
					Port: 5900 + id,
					ID:   id,
					Size: 140,
				}

				if err := web.Templates.ExecuteTemplate(&buf, "screenshot", &vm); err != nil {
					log.Error("unable to execute template screenshot -- %v", err)
					continue
				}
			}

			res := []interface{}{host, template.HTML(buf.String())}
			log.Debug("res: %v", res)
			for _, v := range row {
				res = append(res, v)
			}

			table.Tabular = append(table.Tabular, res)
		}
	}

	webRenderTemplate(w, "table.html", table)
}

func webTileVMs(w http.ResponseWriter, r *http.Request) {
	masks := []string{"id", "name"}
	filters := []string{"state!=error", "state!=quit"}

	vms := []vmScreenshotParams{}
	for host, rows := range globalVmInfo(masks, filters) {
		for _, row := range rows {
			id, err := strconv.Atoi(row[0])
			if err != nil {
				log.Errorln(err)
				continue
			}

			vms = append(vms, vmScreenshotParams{
				Host: host,
				Name: row[1],
				Port: 5900 + id,
				ID:   id,
				Size: 250,
			})
		}
	}

	webRenderTemplate(w, "tiles.html", vms)
}
