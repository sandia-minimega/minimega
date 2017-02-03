// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"flag"
	"fmt"
	"miniclient"
	log "minilog"
	"net/http"
	"path/filepath"

	"golang.org/x/net/websocket"
)

const (
	defaultWebPort = 9001
	defaultWebRoot = "misc/web"
	friendlyError  = "oops, something went wrong"
	BASE_PATH      = "/tmp/minimega"
	banner         = `minimega, Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.`
)

type vmScreenshotParams struct {
	Host string
	Name string
	Port int
	ID   int
	Size int
}

var (
	f_base = flag.String("base", BASE_PATH, "base path for minimega data")
)

var mm *miniclient.Conn

var web struct {
	Running bool
	Server  *http.Server
	Port    int
	Root    string
}

func webStart(port int, root string) {
	web.Root = root

	mux := http.NewServeMux()
	for _, v := range []string{"css", "fonts", "js", "libs", "novnc", "images", "xterm.js"} {
		path := fmt.Sprintf("/%s/", v)
		dir := http.Dir(filepath.Join(root, v))
		mux.Handle(path, http.StripPrefix(path, http.FileServer(dir)))
	}

	mux.HandleFunc("/", indexHandler)

	mux.HandleFunc("/vms", templateHander)
	mux.HandleFunc("/hosts", templateHander)
	mux.HandleFunc("/graph", templateHander)
	mux.HandleFunc("/tilevnc", templateHander)

	mux.HandleFunc("/hosts.json", hostsHandler)
	mux.HandleFunc("/vms.json", vmsHandler)
	mux.HandleFunc("/vlans.json", vlansHandler)

	mux.HandleFunc("/connect/", connectHandler)
	mux.HandleFunc("/screenshot/", screenshotHandler)
	mux.Handle("/tunnel/", websocket.Handler(tunnelHandler))

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

func usage() {
	fmt.Println(banner)
	fmt.Println("usage: miniweb [option]...")
	flag.PrintDefaults()
}

func main() {
	var err error

	flag.Usage = usage
	flag.Parse()

	log.Init()

	mm, err = miniclient.Dial(*f_base)
	if err != nil {
		log.Fatalln(err)
	}

	webStart(9001, "misc/web")
}
