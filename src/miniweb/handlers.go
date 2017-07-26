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
	log "minilog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kr/pty"
	"golang.org/x/net/websocket"
)

func respondJSON(w http.ResponseWriter, data interface{}) {
	js, err := json.Marshal(data)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// indexHandler redirect / to /vms
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, "/vms", 302)
}

// Templated HTML responses
func templateHander(w http.ResponseWriter, r *http.Request) {
	lp := filepath.Join(*f_root, "templates", "_layout.tmpl")
	fp := filepath.Join(*f_root, "templates", r.URL.Path+".tmpl")

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

	if err := tmpl.ExecuteTemplate(w, "layout", nil); err != nil {
		log.Error(err.Error())
		http.Error(w, http.StatusText(500), 500)
	}
}

// screenshotHandler serves routes like /screenshot/<name>.png. Optional size
// query parameter dictates the size of the screenshot.
func screenshotHandler(w http.ResponseWriter, r *http.Request) {
	// URL should be of the form `/screenshot/<name>.png`
	path := strings.Trim(r.URL.Path, "/")

	fields := strings.Split(path, "/")
	if len(fields) != 2 || !strings.HasSuffix(fields[1], ".png") {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimSuffix(fields[1], ".png")

	// TODO: sanitize?
	size := r.URL.Query().Get("size")

	// TODO: replace w with base64 encoder?
	do_encode := r.URL.Query().Get("base64") != ""

	cmd := fmt.Sprintf("vm screenshot %s file /dev/null %s", name, size)

	var screenshot []byte

	for resps := range mm.Run(cmd) {
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

func connectHandler(w http.ResponseWriter, r *http.Request) {
	// URL should be of the form `/connect/<name>`
	fields := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(fields) != 2 {
		return
	}
	name := fields[1]

	// set no-cache headers
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate") // HTTP 1.1.
	w.Header().Set("Pragma", "no-cache")                                   // HTTP 1.0.
	w.Header().Set("Expires", "0")                                         // Proxies.

	var vmType string

	columns := []string{"type"}
	filters := []string{fmt.Sprintf("name=%q", name)}

	for _, vm := range vmInfo(columns, filters) {
		vmType = vm["type"]
	}

	switch vmType {
	case "kvm":
		http.ServeFile(w, r, filepath.Join(*f_root, "vnc.html"))
	case "container":
		http.ServeFile(w, r, filepath.Join(*f_root, "terminal.html"))
	default:
		http.NotFound(w, r)
	}
}

func tunnelHandler(ws *websocket.Conn) {
	// URL should be of the form `/ws/tunnel/<name>`
	path := strings.Trim(ws.Config().Location.Path, "/")

	fields := strings.Split(path, "/")
	if len(fields) != 3 {
		return
	}
	name := fields[2]

	var host string
	var port int

	columns := []string{"host", "type", "vnc_port", "console_port"}
	filters := []string{fmt.Sprintf("name=%q", name)}

	for _, vm := range vmInfo(columns, filters) {
		host = vm["host"]

		switch vm["type"] {
		case "kvm":
			// Undocumented "feature" of websocket -- need to set to
			// PayloadType in order for a direct io.Copy to work.
			ws.PayloadType = websocket.BinaryFrame

			port, _ = strconv.Atoi(vm["vnc_port"])
		case "container":
			// See above. The javascript terminal needs it to be a TextFrame.
			ws.PayloadType = websocket.TextFrame

			port, _ = strconv.Atoi(vm["console_port"])
		default:
			log.Info("unknown VM type: %v", vm["type"])
			return
		}
	}

	if host == "" || port == 0 {
		return
	}

	// connect to the remote host
	rhost := fmt.Sprintf("%v:%v", host, port)
	remote, err := net.Dial("tcp", rhost)
	if err != nil {
		log.Errorln(err)
		return
	}
	defer remote.Close()

	log.Info("ws client connected to %v", rhost)

	go io.Copy(ws, remote)
	io.Copy(remote, ws)

	log.Info("ws client disconnected from %v", rhost)
}

func vmsHandler(w http.ResponseWriter, r *http.Request) {
	var vms []map[string]string

	if strings.HasSuffix(r.URL.Path, "/info.json") {
		// don't care about quit or error state
		vms = vmInfo(nil, []string{
			"state!=quit",
			"state!=error",
		})
	} else if strings.HasSuffix(r.URL.Path, "/top.json") {
		vms = vmTop(nil, nil)
	} else {
		http.NotFound(w, r)
		return
	}

	sortVMs(vms)
	respondJSON(w, vms)
}

func hostsHandler(w http.ResponseWriter, r *http.Request) {
	hosts := [][]interface{}{}

	cmd := "host"

	for resps := range mm.Run(cmd) {
		for _, resp := range resps.Resp {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			for _, row := range resp.Tabular {
				res := []interface{}{}
				for _, v := range row {
					res = append(res, v)
				}
				hosts = append(hosts, res)
			}
		}
	}

	respondJSON(w, hosts)
}

func vlansHandler(w http.ResponseWriter, r *http.Request) {
	vlans := [][]interface{}{}

	cmd := "vlans"

	for resps := range mm.Run(cmd) {
		for _, resp := range resps.Resp {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			for _, row := range resp.Tabular {
				res := []interface{}{}
				for _, v := range row {
					res = append(res, v)
				}
				vlans = append(vlans, res)
			}
		}
	}

	respondJSON(w, vlans)
}

func consoleHandler(ws *websocket.Conn) {
	// start minimega in a new pty
	cmd := exec.Command("bin/minimega", "-attach")

	tty, err := pty.Start(cmd)
	if err != nil {
		log.Error("start failed:", err)
		return
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Process.Wait()
		tty.Close()
	}()

	go io.Copy(ws, tty)
	io.Copy(tty, ws)

	cmd.Process.Kill()

	if _, err := cmd.Process.Wait(); err != nil {
		log.Error("wait failed: %v", err)
	}
}
