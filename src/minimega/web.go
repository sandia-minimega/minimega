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
	"path/filepath"
	"sort"
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
	Toggle  map[string]int
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
	Port      int
}

var webCLIHandlers = []minicli.Handler{
	{ // web
		HelpShort: "start the minimega webserver",
		HelpLong: `
Launch the minimega webserver. Running web starts the HTTP server whose port
cannot be changed once started. The default port is 9001. To run the server on
a different port, run:

	web 10000

The webserver requires several resources found in misc/web in the repo. By
default, it looks in $PWD/misc/web for these resources. If you are running
minimega from a different location, you can specify a different path using:

	web root <path/to/web/dir>

You can also set the port when starting web with an alternative root directory:

	web root <path/to/web/dir> 10000

NOTE: If you start the webserver with an invalid root, you can safely re-run
"web root" to update it. You cannot, however, change the server's port.`,
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

	templates := filepath.Join(root, "templates", "*.html")
	log.Info("compiling templates from %s", templates)
	web.Templates, err = template.ParseGlob(templates)
	if err != nil {
		log.Error("failed to load templates from %s", templates)
		return
	}

	mux := http.NewServeMux()
	for _, v := range []string{"novnc", "d3", "include"} {
		path := fmt.Sprintf("/%s/", v)
		dir := http.Dir(filepath.Join(root, v))
		mux.Handle(path, http.StripPrefix(path, http.FileServer(dir)))
	}

	mux.HandleFunc("/", webVMs)
	mux.HandleFunc("/map", webMapVMs)
	mux.HandleFunc("/screenshot/", webScreenshot)
	mux.HandleFunc("/hosts", webHosts)
	mux.HandleFunc("/tags", webVMTags)
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
			log.Error("web: %v", err)
			web.Server = nil
		} else {
			web.Port = port
			web.Running = true
		}
	} else {
		log.Info("web: changing web root to: %s", root)
		if port != web.Port && port != defaultWebPort {
			log.Error("web: changing web's port is not supported")
		}
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

	data := struct {
		Title, Path string
	}{
		Title: fmt.Sprintf("%s:%s", host, vm),
		Path:  fmt.Sprintf("ws/%s/%s", host, port),
	}

	webRenderTemplate(w, "vnc.html", data)
}

func webMapVMs(w http.ResponseWriter, r *http.Request) {
	var err error

	type point struct {
		Lat, Long float64
		Text      string
	}

	points := []point{}

	info, _ := globalVmInfo(nil, nil)
	for _, vms := range info {
		for _, vm := range vms {
			name := fmt.Sprintf("%v:%v", vm.ID(), vm.Name())

			p := point{Text: name}

			if vm.Tag("lat") == "" || vm.Tag("long") == "" {
				log.Debug("skipping vm %s -- missing required tags lat/long", name)
				continue
			}

			p.Lat, err = strconv.ParseFloat(vm.Tag("lat"), 64)
			if err != nil {
				log.Error("invalid lat for vm %s -- expected float")
				continue
			}

			p.Long, err = strconv.ParseFloat(vm.Tag("lat"), 64)
			if err != nil {
				log.Error("invalid lat for vm %s -- expected float")
				continue
			}

			points = append(points, p)
		}
	}

	webRenderTemplate(w, "map.html", points)
}

func webVMTags(w http.ResponseWriter, r *http.Request) {
	table := htmlTable{
		Header:  []string{},
		Toggle:  map[string]int{},
		Tabular: [][]interface{}{},
	}

	tags := map[string]bool{}

	info, _ := globalVmInfo(nil, nil)

	// Find all the distinct tags across all VMs
	for _, vms := range info {
		for _, vm := range vms {
			for _, k := range vm.Tags() {
				tags[k] = true
			}
		}
	}

	fixedCols := []string{"Host", "Name", "ID"}

	// Copy into Header
	for k := range tags {
		table.Header = append(table.Header, k)
	}
	sort.Strings(table.Header)

	// Set up Toggle, offset by fixedCols which will be on the left
	for i, v := range table.Header {
		table.Toggle[v] = i + len(fixedCols)
	}

	// Update the VM's tags so that it contains all the distinct values and
	// then populate data
	for host, vms := range info {
		for _, vm := range vms {
			row := []interface{}{
				host,
				vm.Name(),
				vm.ID(),
			}

			for _, k := range table.Header {
				// If key is not present, will set it to the zero-value
				row = append(row, vm.Tag(k))
			}

			table.Tabular = append(table.Tabular, row)
		}
	}

	// Add "fixed" headers for host/...
	table.Header = append(fixedCols, table.Header...)

	webRenderTemplate(w, "tags.html", table)
}

func webHosts(w http.ResponseWriter, r *http.Request) {
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

	webRenderTemplate(w, "hosts.html", table)
}

func webVMs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// HAX: Part of code to hack around "dynamic" fields in vm info.
	findHeader := func(needle string, header []string) (int, error) {
		for i, v := range header {
			if v == needle {
				return i, nil
			}
		}
		return 0, fmt.Errorf("header `%s` not found", needle)
	}

	table := htmlTable{
		Header:  []string{"host", "screenshot"},
		Tabular: [][]interface{}{},
		ID:      "example",
		Class:   "hover",
	}
	table.Header = append(table.Header, vmMasks...)

	stateMask := VM_QUIT | VM_ERROR

	info, raw := globalVmInfo(nil, nil)
	for host, vms := range info {
		for _, vm := range vms {
			var buf bytes.Buffer
			if vm.State()&stateMask == 0 {
				params := vmScreenshotParams{
					Host: host,
					Name: vm.Name(),
					Port: 5900 + vm.ID(),
					ID:   vm.ID(),
					Size: 140,
				}

				if err := web.Templates.ExecuteTemplate(&buf, "screenshot", &params); err != nil {
					log.Error("unable to execute template screenshot -- %v", err)
					continue
				}
			}

			res := []interface{}{host, template.HTML(buf.String())}

			row, err := vm.Info(vmMasks)
			if err != nil {
				log.Error("unable to get info from VM %s:%s -- %v", host, vm.Name(), err)
				continue
			}

			// HAX: Patch up "dynamic" fields from tabular data. This will be
			// deleted when we track all the VM state in the VM struct.
			for i, v := range vmMasks {
				id := fmt.Sprintf("%v", vm.ID())
				switch v {
				case "ip", "ip6", "cc_active":
					log.Debug("patching `%s` field for `%s` host", v, host)
					for _, resp := range raw[host] {
						idIdx, err := findHeader("id", resp.Header)
						if err != nil {
							log.Debug("%v", err)
							continue
						}

						vIdx, err := findHeader(v, resp.Header)
						if err != nil {
							log.Debug("%v", err)
							continue
						}

						for _, r := range resp.Tabular {
							if id == r[idIdx] {
								row[i] = r[vIdx]
							}
						}
					}
				}

				res = append(res, row[i])
			}

			table.Tabular = append(table.Tabular, res)
		}
	}

	webRenderTemplate(w, "table.html", table)
}

func webTileVMs(w http.ResponseWriter, r *http.Request) {
	stateMask := VM_QUIT | VM_ERROR

	params := []vmScreenshotParams{}

	info, _ := globalVmInfo(nil, nil)
	for host, vms := range info {
		for _, vm := range vms {
			if vm.State()&stateMask != 0 {
				continue
			}

			params = append(params, vmScreenshotParams{
				Host: host,
				Name: vm.Name(),
				Port: 5900 + vm.ID(),
				ID:   vm.ID(),
				Size: 250,
			})
		}
	}

	webRenderTemplate(w, "tiles.html", params)
}
