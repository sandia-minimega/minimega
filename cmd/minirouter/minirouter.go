// minirouter is a simple wrapper daemon for bird, dnsmasq, and iptool. It listens on a
// unix domain socket for updates and in turn updates configs for downstream
// services. minirouter also gathers statistics to populate vm tags on the host
// minimega instance and for use by the 'router' API in minimega.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	f_miniccc = flag.String("miniccc", "/miniccc", "path to miniccc for sending logging and stats to minimega")
	f_path    = flag.String("path", "/tmp/minirouter", "base directory for minirouter")
	f_force   = flag.Bool("force", false, "force minirouter to run even if another appears to be running already")
	f_u       = flag.String("u", "", "update minirouter with a given file")
	f_cli     = flag.Bool("cli", false, "validate and print the minirouter cli, in JSON, to stdout and exit")
)

func main() {
	// flags
	flag.Parse()

	log.Init()
	logSetupPushUp()

	if *f_cli {
		if err := minicli.Validate(); err != nil {
			log.Fatalln(err)
		}

		doc, err := minicli.Doc()
		if err != nil {
			log.Fatal("failed to generate docs: %v", err)
		}
		fmt.Println(doc)
		os.Exit(0)
	}

	if *f_u != "" {
		log.Debug("updating with file: %v", *f_u)

		err := update(filepath.Join(*f_path, "minirouter"), *f_u)
		if err != nil {
			log.Errorln(err)
		}

		return
	}

	// check for a running instance of minirouter
	_, err := os.Stat(filepath.Join(*f_path, "minirouter"))
	if err == nil {
		if !*f_force {
			log.Fatalln("minirouter appears to already be running, override with -force")
		}
		log.Warn("minirouter may already be running, proceed with caution")
		err = os.Remove(filepath.Join(*f_path, "minirouter"))
		if err != nil {
			log.Fatalln(err)
		}
	}

	log.Debug("using path: %v", *f_path)

	// attempt to set up the base path
	err = os.MkdirAll(*f_path, os.FileMode(0770))
	if err != nil {
		log.Fatal("mkdir base path: %v", err)
	}

	// start the domain socket service
	go commandSocketStart()

	// signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	<-sig

	// cleanup
	err = os.Remove(filepath.Join(*f_path, "minirouter"))
	if err != nil {
		log.Fatalln(err)
	}
}
