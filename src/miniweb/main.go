// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"miniclient"
	log "minilog"
	"net/http"
	"path/filepath"

	"golang.org/x/net/websocket"
)

const (
	defaultAddr = ":9001"
	defaultRoot = "misc/web"
	defaultBase = "/tmp/minimega"
)

const banner = `miniweb, Copyright (2017) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.`

var (
	f_addr = flag.String("addr", defaultAddr, "listen address")
	f_root = flag.String("root", defaultRoot, "base path for web files")
	f_base = flag.String("base", defaultBase, "base path for minimega")
)

var mm *miniclient.Conn

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

	files, err := ioutil.ReadDir(*f_root)
	if err != nil {
		log.Fatalln(err)
	}

	mux := http.NewServeMux()

	for _, f := range files {
		if f.IsDir() {
			path := fmt.Sprintf("/%s/", f.Name())
			dir := http.Dir(filepath.Join(*f_root, f.Name()))
			mux.Handle(path, http.StripPrefix(path, http.FileServer(dir)))
		}
	}

	mux.HandleFunc("/", indexHandler)

	mux.HandleFunc("/vms", templateHander)
	mux.HandleFunc("/hosts", templateHander)
	mux.HandleFunc("/graph", templateHander)
	mux.HandleFunc("/tilevnc", templateHander)

	mux.HandleFunc("/hosts.json", hostsHandler)
	mux.HandleFunc("/vlans.json", vlansHandler)
	mux.HandleFunc("/vms/info.json", vmsHandler)
	mux.HandleFunc("/vms/top.json", vmsHandler)

	mux.HandleFunc("/connect/", connectHandler)
	mux.HandleFunc("/screenshot/", screenshotHandler)
	mux.Handle("/ws/tunnel/", websocket.Handler(tunnelHandler))

	mux.HandleFunc("/console", templateHander)
	mux.Handle("/ws/console/", websocket.Handler(consoleHandler))

	server := &http.Server{
		Addr:    *f_addr,
		Handler: mux,
	}

	log.Fatalln(server.ListenAndServe())
}
