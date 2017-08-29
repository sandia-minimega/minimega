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
	f_addr      = flag.String("addr", defaultAddr, "listen address")
	f_root      = flag.String("root", defaultRoot, "base path for web files")
	f_base      = flag.String("base", defaultBase, "base path for minimega")
	f_passwords = flag.String("passwords", "", "password file for auth")
	f_bootstrap = flag.Bool("bootstrap", false, "create password file for auth")
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

	if *f_bootstrap {
		if *f_passwords == "" {
			log.Fatalln("must specify -password for bootstrap")
		}

		if err := bootstrap(*f_passwords); err != nil {
			log.Fatalln(err)
		}

		return
	}

	if *f_passwords != "" {
		if err := parsePasswords(*f_passwords); err != nil {
			log.Fatalln(err)
		}
	}

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

	mux.HandleFunc("/", mustAuth(indexHandler))

	mux.HandleFunc("/vms", mustAuth(templateHander))
	mux.HandleFunc("/hosts", mustAuth(templateHander))
	mux.HandleFunc("/graph", mustAuth(templateHander))
	mux.HandleFunc("/tilevnc", mustAuth(templateHander))

	mux.HandleFunc("/hosts.json", mustAuth(hostsHandler))
	mux.HandleFunc("/vlans.json", mustAuth(vlansHandler))
	mux.HandleFunc("/vms/info.json", mustAuth(vmsHandler))
	mux.HandleFunc("/vms/top.json", mustAuth(vmsHandler))

	mux.HandleFunc("/connect/", mustAuth(connectHandler))
	mux.HandleFunc("/screenshot/", mustAuth(screenshotHandler))

	server := &http.Server{
		Addr:    *f_addr,
		Handler: mux,
	}

	log.Fatalln(server.ListenAndServe())
}
