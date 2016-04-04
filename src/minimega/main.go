// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"flag"
	"fmt"
	"goreadline"
	"minicli"
	"miniclient"
	log "minilog"
	"minipager"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"version"
)

const (
	BASE_PATH = "/tmp/minimega"
	IOM_PATH  = "/tmp/minimega/files"
	Wildcard  = "all"
	Localhost = "localhost"
)

var (
	f_loglevel   = flag.String("level", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log        = flag.Bool("v", true, "log on stderr")
	f_logfile    = flag.String("logfile", "", "also log to file")
	f_base       = flag.String("base", BASE_PATH, "base path for minimega data")
	f_e          = flag.Bool("e", false, "execute command on running minimega")
	f_degree     = flag.Int("degree", 0, "meshage starting degree")
	f_msaTimeout = flag.Int("msa", 10, "meshage MSA timeout")
	f_port       = flag.Int("port", 9000, "meshage port to listen on")
	f_ccPort     = flag.Int("ccport", 9002, "cc port to listen on")
	f_force      = flag.Bool("force", false, "force minimega to run even if it appears to already be running")
	f_nostdin    = flag.Bool("nostdin", false, "disable reading from stdin, useful for putting minimega in the background")
	f_version    = flag.Bool("version", false, "print the version and copyright notices")
	f_nameshpace = flag.String("nameshpace", "minimega", "meshage namespace for discovery")
	f_iomBase    = flag.String("filepath", IOM_PATH, "directory to serve files from")
	f_attach     = flag.Bool("attach", false, "attach the minimega command line to a running instance of minimega")
	f_cli        = flag.Bool("cli", false, "validate and print the minimega cli, in markdown, to stdout and exit")
	f_panic      = flag.Bool("panic", false, "panic on quit, producing stack traces for debugging")

	vms      VMs
	hostname string
	reserved = []string{Wildcard}
)

const (
	banner = `minimega, Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.`
	poeticDeath = `Willst du immer weiterschweifen?
Sieh, das Gute liegt so nah.
Lerne nur das Glück ergreifen,
denn das Glück ist immer da.`
)

func usage() {
	fmt.Println(banner)
	fmt.Println("usage: minimega [option]... [file]...")
	flag.PrintDefaults()
}

func main() {
	var err error

	// see containerShim()
	if len(os.Args) > 1 && os.Args[1] == CONTAINER_MAGIC {
		containerShim()
	}

	flag.Usage = usage
	flag.Parse()

	logSetup()
	cliSetup()

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

	// rebase f_iomBase if f_base changed but iomBase did not
	if *f_base != BASE_PATH && *f_iomBase == IOM_PATH {
		*f_iomBase = filepath.Join(*f_base, "files")
	}

	if *f_version {
		fmt.Println("minimega", version.Revision, version.Date)
		fmt.Println(version.Copyright)
		os.Exit(0)
	}

	hostname, err = os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	if isReserved(hostname) {
		log.Warn("hostname `%s` is a reserved word -- abandon all hope, ye who enter here", hostname)
	}

	vms = make(map[int]VM)

	// special case, catch -e and execute a command on an already running
	// minimega instance
	if *f_e || *f_attach {
		// try to connect to the local minimega
		mm, err := miniclient.Dial(*f_base)
		if err != nil {
			log.Fatalln(err)
		}
		mm.Pager = minipager.DefaultPager

		if *f_e {
			a := flag.Args()
			log.Debugln("got args:", a)

			// TODO: Need to escape?
			cmd := minicli.MustCompile(strings.Join(a, " "))
			log.Infoln("got command:", cmd)

			mm.RunAndPrint(cmd, false)
		} else {
			mm.Attach()
		}

		return
	}

	// warn if we're not root
	user, err := user.Current()
	if err != nil {
		log.Fatalln(err)
	}
	if user.Uid != "0" {
		log.Warnln("not running as root")
	}

	// check for a running instance of minimega
	_, err = os.Stat(filepath.Join(*f_base, "minimega"))
	if err == nil {
		if !*f_force {
			log.Fatalln("minimega appears to already be running, override with -force")
		}
		log.Warn("minimega may already be running, proceed with caution")
		err = os.Remove(filepath.Join(*f_base, "minimega"))
		if err != nil {
			log.Fatalln(err)
		}
	}

	// set up signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		first := true
		for {
			<-sig
			if *f_panic {
				panic("teardown")
			}
			if first {
				log.Info("caught signal, tearing down, ctrl-c again will force quit")
				go teardown()
				first = false
			} else {
				os.Exit(1)
			}
		}
	}()

	err = checkExternal()
	if err != nil {
		log.Warnln(err.Error())
	}

	// attempt to set up the base path
	err = os.MkdirAll(*f_base, os.FileMode(0770))
	if err != nil {
		log.Fatal("mkdir base path: %v", err)
	}

	pid := os.Getpid()
	writeOrDie(filepath.Join(*f_base, "minimega.pid"), strconv.Itoa(pid))

	go commandSocketStart()

	// create a node for meshage
	host, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}
	meshageInit(host, *f_nameshpace, uint(*f_degree), *f_port)

	// start the cc service
	ccStart()

	fmt.Println(banner)

	// fan out to the number of cpus on the system if GOMAXPROCS env variable is
	// not set.
	if os.Getenv("GOMAXPROCS") == "" {
		cpus := runtime.NumCPU()
		runtime.GOMAXPROCS(cpus)
	}

	if !*f_nostdin {
		cliLocal()
	} else {
		<-sig
		if *f_panic {
			panic("teardown")
		}
	}
	teardown()
}

func teardownf(format string, args ...interface{}) {
	log.Error(format, args...)

	teardown()
}

func teardown() {
	// Clear namespace so that we hit all the VMs
	namespace = ""

	vncClear()
	clearAllCaptures()
	vms.kill(Wildcard)
	dnsmasqKillAll()
	ksmDisable()
	vms.flush()
	vms.cleanDirs()
	containerTeardown()

	if err := bridgesDestroy(); err != nil {
		log.Errorln(err)
	}

	commandSocketRemove()
	goreadline.Rlcleanup()

	if err := os.Remove(filepath.Join(*f_base, "minimega.pid")); err != nil {
		log.Fatalln(err)
	}

	if cpuProfileOut != nil {
		pprof.StopCPUProfile()
		cpuProfileOut.Close()
	}

	os.Exit(0)
}
