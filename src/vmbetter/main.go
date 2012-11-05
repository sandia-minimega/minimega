package main

import (
	"flag"
	"fmt"
	log "minilog"
	"os"
	"vmconfig"
)

var (
	f_loglevel = flag.String("level", "error", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
)

var banner string = `vmbetter, Copyright 2012 Sandia Corporation.
vmbetter comes with ABSOLUTELY NO WARRANTY. This is free software, and you are
welcome to redistribute it under certain conditions. See the included LICENSE
for details.
`

func usage() {
	fmt.Println(banner)
	fmt.Println("usage: vmbetter [option]... [config]")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	log_setup()

	if flag.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	// find any other dependent configs and get an ordered list of those 
	handle_config(flag.Arg(0))

	configfile := flag.Arg(0)
	log.Debugln("using config:", configfile)
	m, err := vmconfig.ReadConfig(configfile)
	if err != nil {
		log.Fatalln(err)
	} else {
		log.Debugln("read config:", m)
	}

	// merge packages to add from all dependent configs
	// invoke debootstrap
	// copy the default init script over
	// copy any overlay into place in reverse order of opened dependencies
	// call post build chroot commands in reverse order as well
	// build the image file
	// copy out the kernel 
	// cleanup?
}

func log_setup() {
	level, err := log.LevelInt(*f_loglevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *f_log {
		log.AddLogger("stdio", os.Stderr, level, true)
	}

	if *f_logfile != "" {
		logfile, err := os.OpenFile(*f_logfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		log.AddLogger("file", logfile, level, false)
	}
}
