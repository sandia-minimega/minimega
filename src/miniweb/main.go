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
	f_console   = flag.Bool("console", false, "enable console")
	f_key       = flag.String("key", "", "key file for TLS in PEM format")
	f_cert      = flag.String("cert", "", "cert file for TLS in PEM format")
	f_namespace = flag.String("namespace", "", "limit miniweb to a namespace")
)

var mm *miniclient.Conn

func usage() {
	fmt.Println(banner)
	fmt.Println("usage: miniweb [option]...")
	flag.PrintDefaults()
}

var mux = http.NewServeMux()

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

	for _, f := range files {
		if f.IsDir() {
			path := fmt.Sprintf("/%s/", f.Name())
			dir := http.Dir(filepath.Join(*f_root, f.Name()))
			mux.Handle(path, http.StripPrefix(path, http.FileServer(dir)))
		}
	}

	mux.HandleFunc("/", mustAuth(indexHandler))

	mux.HandleFunc("/vms", mustAuth(templateHandler))
	mux.HandleFunc("/hosts", mustAuth(templateHandler))
	mux.HandleFunc("/graph", mustAuth(templateHandler))
	mux.HandleFunc("/tilevnc", mustAuth(templateHandler))

	mux.HandleFunc("/hosts.json", mustAuth(tabularHandler))
	mux.HandleFunc("/vlans.json", mustAuth(tabularHandler))
	mux.HandleFunc("/vms/info.json", mustAuth(vmsHandler))
	mux.HandleFunc("/vms/top.json", mustAuth(vmsHandler))

	mux.HandleFunc("/connect/", mustAuth(connectHandler))
	mux.HandleFunc("/screenshot/", mustAuth(screenshotHandler))

	if *f_console {
		mux.HandleFunc("/console", mustAuth(consoleHandler))
		mux.HandleFunc("/console/", mustAuth(consoleHandler))
	} else {
		mux.HandleFunc("/console", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "console disabled, see -console flag", http.StatusNotImplemented)
			return
		})
	}

	server := &http.Server{
		Addr:    *f_addr,
		Handler: mux,
	}

	if *f_cert != "" && *f_key != "" {
		log.Info("serving HTTPS on %v", *f_addr)
		log.Fatalln(server.ListenAndServeTLS(*f_cert, *f_key))
	}
	if *f_cert != "" || *f_key != "" {
		log.Fatalln("must specify both cert and key files to enable TLS")
	}

	log.Info("serving HTTP on %v", *f_addr)
	log.Fatalln(server.ListenAndServe())
}
