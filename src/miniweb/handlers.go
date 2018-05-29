// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"minicli"
	log "minilog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/kr/pty"
	"golang.org/x/net/websocket"
)

var ptys = map[int]*os.File{}
var ptyMu sync.Mutex

func respondJSON(w http.ResponseWriter, data interface{}) {
	js, err := json.Marshal(data)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// indexHandler handles all unmatched URLs, redirects / to /vms
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/vms", 302)
		return
	}

	// don't mess with this if we are tied to a namespace
	if *f_namespace != "" {
		http.NotFound(w, r)
		return
	}

	// potentially prefixed with a namespace
	log.Debug("URL: %v", r.URL)

	// split URL into <namespace>/<rest of URL>
	path := strings.TrimPrefix(r.URL.Path, "/")
	fields := strings.SplitN(path, "/", 2)

	// only have a possible namespace -- redirect
	if len(fields) == 1 {
		http.Redirect(w, r, path+"/", 302)
		return
	}

	// add namespace to query values
	v := r.URL.Query()
	if v.Get("namespace") != "" {
		// something strange is going on
		http.NotFound(w, r)
		return
	}
	v.Set("namespace", fields[0])

	// patch up query and hand back to the mux
	r.URL.RawQuery = v.Encode()
	r.URL.Path = "/" + fields[1]

	log.Debug("new URL: %v", r.URL)

	mux.ServeHTTP(w, r)
}

func renderTemplate(w http.ResponseWriter, r *http.Request, t string, d interface{}) {
	lp := filepath.Join(*f_root, "templates", "_layout.tmpl")
	fp := filepath.Join(*f_root, "templates", t)

	info, err := os.Stat(fp)
	if err != nil {
		// 404 if template doesn't exist
		http.NotFound(w, r)
		return
	}

	if info.IsDir() {
		// 404 if directory
		http.NotFound(w, r)
		return
	}

	tmpl, err := template.ParseFiles(lp, fp)
	if err != nil {
		log.Error(err.Error())
		http.Error(w, http.StatusText(500), 500)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "layout", d); err != nil {
		log.Error(err.Error())
		http.Error(w, http.StatusText(500), 500)
	}
}

// Templated HTML responses
func templateHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, r, r.URL.Path+".tmpl", nil)
}

// filesHandler ignores subpaths and renders the files template
func filesHandler(w http.ResponseWriter, r *http.Request) {
	log.Info("files handler: %v", r.URL.Path)

	renderTemplate(w, r, "files.tmpl", nil)
}

// screenshotHandler handles the following URLs via vmHandler:
//   /vm/<name>/screenshot.png
func screenshotHandler(w http.ResponseWriter, r *http.Request, name string) {
	log.Info("screenshotHandler handler: %v", r.URL.Path)

	// TODO: sanitize?
	size := r.URL.Query().Get("size")

	// TODO: replace w with base64 encoder?
	do_encode := r.URL.Query().Get("base64") != ""

	cmd := NewCommand(r)
	cmd.Command = fmt.Sprintf("vm screenshot %s file /dev/null %s", name, size)

	var screenshot []byte

	for resps := range run(cmd) {
		for _, resp := range resps.Resp {
			if resp.Error != "" {
				if strings.HasPrefix(resp.Error, "vm not running:") {
					continue
				} else if resp.Error == "cannot take screenshot of container" {
					continue
				}

				// Unknown error
				log.Errorln(resp.Error)
				http.Error(w, "unknown error", http.StatusInternalServerError)
				return
			}

			if resp.Data == nil {
				log.Info("no data")
				http.NotFound(w, r)
				return
			}

			if screenshot == nil {
				screenshot, _ = base64.StdEncoding.DecodeString(resp.Data.(string))
			} else {
				log.Error("received more than one response for vm screenshot")
			}
		}
	}

	if screenshot == nil {
		http.NotFound(w, r)
		return
	}

	if do_encode {
		base64string := "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
		w.Write([]byte(base64string))
	} else {
		w.Write(screenshot)
	}
}

// connectHandler handles the following URLs via vmHandler:
//   /vm/<name>/connect/
//   /vm/<name>/connect/ws
func connectHandler(w http.ResponseWriter, r *http.Request, name string) {
	log.Info("connect request: %v", r.URL.Path)

	// find info about the VM that we need to connect
	var vmType string
	var host string
	var port int

	cmd := NewCommand(r)
	cmd.Command = "vm info"
	cmd.Columns = []string{"host", "type", "vnc_port", "console_port"}
	cmd.Filters = []string{fmt.Sprintf("name=%q", name)}

	for _, vm := range runTabular(cmd) {
		host = vm["host"]
		vmType = vm["type"]

		switch vm["type"] {
		case "kvm":
			port, _ = strconv.Atoi(vm["vnc_port"])
		case "container":
			port, _ = strconv.Atoi(vm["console_port"])
		default:
			log.Info("unknown VM type: %v", vm["type"])
			return
		}
	}

	if vmType == "" || host == "" || port == 0 {
		http.NotFound(w, r)
		return
	}

	// check the request again to decide whether to serve the page or tunnel
	// the request
	if strings.HasSuffix(r.URL.Path, "/ws") {
		websocket.Handler(connectWsHandler(vmType, host, port)).ServeHTTP(w, r)

		return
	}

	// set no-cache headers
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate") // HTTP 1.1.
	w.Header().Set("Pragma", "no-cache")                                   // HTTP 1.0.
	w.Header().Set("Expires", "0")                                         // Proxies.

	switch vmType {
	case "kvm":
		http.ServeFile(w, r, filepath.Join(*f_root, "vnc.html"))
	case "container":
		http.ServeFile(w, r, filepath.Join(*f_root, "terminal.html"))
	}
}

// vmHandler handles the following URLs:
//   /vm/<name>/connect/
//   /vm/<name>/connect/ws
//   /vm/<name>/screenshot.png
//   POST /vm/<name>/start
//   POST /vm/<name>/stop
//   POST /vm/<name>/kill
func vmHandler(w http.ResponseWriter, r *http.Request) {
	log.Info("vm request: %v", r.URL.Path)

	fields := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(fields) < 3 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	name := fields[1]

	switch fields[2] {
	case "connect":
		if len(fields) == 3 || len(fields) == 4 {
			connectHandler(w, r, name)
			return
		}
	case "screenshot.png":
		if len(fields) == 3 {
			screenshotHandler(w, r, name)
			return
		}
	case "start", "stop", "kill":
		if r.Method == http.MethodPost && len(fields) == 3 {
			cmd := NewCommand(r)
			cmd.Command = fmt.Sprintf("vm %v %q", fields[2], name)

			var res string
			for resps := range run(cmd) {
				res += resps.Rendered
			}

			w.Write([]byte(res))
			return
		}
	}

	http.NotFound(w, r)
	return
}

// vmsHandler handles the following URLs:
//   /vms/info.json
//   /vms/top.json
func vmsHandler(w http.ResponseWriter, r *http.Request) {
	log.Info("vms request: %v", r.URL)

	fields := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(fields) != 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	var vms []map[string]string

	cmd := NewCommand(r)

	switch fields[1] {
	case "info.json":
		cmd.Command = "vm info"
		// don't care about quit or error state
		cmd.Filters = []string{
			"state!=quit",
			"state!=error",
		}
	case "top.json":
		cmd.Command = "vm top"
	default:
		http.NotFound(w, r)
		return
	}

	vms = runTabular(cmd)
	sortVMs(vms)
	respondJSON(w, vms)
}

// tabularHandler handles the following URLs:
//   /vlans.json
//   /hosts.json
//   /namespaces.json
func tabularHandler(w http.ResponseWriter, r *http.Request) {
	cmd := NewCommand(r)

	switch strings.Trim(r.URL.Path, "/") {
	case "vlans.json":
		cmd.Command = "vlans"
	case "hosts.json":
		cmd.Command = "host"
	case "namespaces.json":
		cmd.Command = "namespace"
	case "files.json":
		path := r.URL.Query().Get("path")
		cmd.Command = fmt.Sprintf("ns run file list %q", path)
	default:
		http.NotFound(w, r)
		return
	}

	respondJSON(w, runTabular(cmd))
}

// consoleHandler handles the following URLs:
//   /console
//   /console/<pid>/ws
//   /console/<pid>/size
//
// Must have -console=true to enable.
func consoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/console" {
		// create a new console
		cmd := exec.Command("bin/minimega", "-attach")

		tty, err := pty.Start(cmd)
		if err != nil {
			log.Error("start failed:", err)
			return
		}

		pid := cmd.Process.Pid

		log.Info("spawned new minimega console, pid = %v", pid)

		ptyMu.Lock()
		defer ptyMu.Unlock()
		ptys[pid] = tty

		data := struct{ Pid int }{
			Pid: pid,
		}
		renderTemplate(w, r, "console.tmpl", &data)
		return
	}

	path := strings.Split(r.URL.Path, "/")

	if len(path) != 4 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	pid, err := strconv.Atoi(path[2])
	if err != nil {
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	ptyMu.Lock()
	tty, ok := ptys[pid]
	if !ok {
		http.Error(w, "pty not found", http.StatusNotFound)
		return
	}
	ptyMu.Unlock()

	switch path[3] {
	case "size":
		rows, err := strconv.ParseUint(r.FormValue("rows"), 10, 16)
		cols, err2 := strconv.ParseUint(r.FormValue("cols"), 10, 16)
		if err != nil || err2 != nil {
			http.Error(w, "invalid rows/cols", http.StatusBadRequest)
			return
		}

		log.Info("resize %v to %vx%x", pid, cols, rows)

		ws := struct {
			R, C, X, Y uint16
		}{
			R: uint16(rows), C: uint16(cols),
		}
		_, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL,
			tty.Fd(),
			syscall.TIOCSWINSZ,
			uintptr(unsafe.Pointer(&ws)),
		)
		if errno != 0 {
			log.Error("unable to set winsize: %v", syscall.Errno(errno))
			http.Error(w, "set winsize failed", http.StatusInternalServerError)
		}

		// make sure winsize gets processed, hopefully the user isn't typing...
		time.Sleep(100 * time.Millisecond)
		io.WriteString(tty, "\n")
		return
	case "ws":
		// run this in a separate goroutine so that we unlock ptyMu
		websocket.Handler(consoleWsHandler(tty, pid)).ServeHTTP(w, r)

		return
	}
}

// commandHandler handles the following URLs:
//   /command
//
// Example usage:
//   curl 'http://localhost:9001/command' -d '{
//   	"command": "vm info"
//   }'
//
//   curl 'http://localhost:9001/command' -d '{
//   	"command": "vm info",
//   	"columns": ["name", "hostname"],
//   	"filters": ["state=building"]
//   }'
//
// Must have -console=true to enable.
func commandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "must use POST", http.StatusMethodNotAllowed)
		return
	}

	cmd := NewCommand(r)
	if err := json.NewDecoder(r.Body).Decode(cmd); err != nil {
		log.Error("unable to parse body: %v", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if cmd.Command == "" {
		http.Error(w, "must specify command", http.StatusBadRequest)
		return
	}

	resps := []minicli.Responses{}

	for resp := range run(cmd) {
		resps = append(resps, resp.Resp)
	}

	respondJSON(w, resps)
}
