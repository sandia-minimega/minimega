// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// vmbetter is a debootstrap based toolchain for creating virtual machine
// images for booting host and guest nodes. It generates a debian based initrd
// and kernel image from a configuration file.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"vmconfig"
)

var (
	f_loglevel      = flag.String("level", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log           = flag.Bool("v", true, "log on stderr")
	f_logfile       = flag.String("logfile", "", "also log to file")
	f_debian_mirror = flag.String("mirror", "http://ftp.us.debian.org/debian", "path to the debian mirror to use")
	f_noclean       = flag.Bool("noclean", false, "do not remove build directory")
	f_stage1        = flag.Bool("1", false, "stop after stage one, and copy build files to <config>_stage1")
	f_stage2        = flag.String("2", "", "complete stage 2 from an existing stage 1 directory")
	f_branch        = flag.String("branch", "testing", "debian branch to use")
	f_qcow          = flag.Bool("qcow", false, "generate a qcow2 image instead of a kernel/initrd pair")
	f_qcowsize      = flag.String("qcowsize", "1G", "qcow2 image size (eg 1G, 1024M)")
)

var banner string = `vmbetter, Copyright 2012 Sandia Corporation.
vmbetter comes with ABSOLUTELY NO WARRANTY. This is free software, and you are
welcome to redistribute it under certain conditions. See the included LICENSE
for details.
`

// usage prints the flag usage parameters.
func usage() {
	fmt.Println(banner)
	fmt.Println("usage: vmbetter [option]... [config]")
	flag.PrintDefaults()
}

// TODO(fritz): make vmbetter use external.go style process lookups throughout

func main() {
	flag.Usage = usage
	flag.Parse()

	LogSetup()

	if flag.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	// stage 1 and stage 2 flags are mutually exclusive
	if *f_stage1 && *f_stage2 != "" {
		log.Fatalln("-1 cannot be used with -2")
	}

	// find any other dependent configs and get an ordered list of those
	configfile := flag.Arg(0)
	log.Debugln("using config:", configfile)
	config, err := vmconfig.ReadConfig(configfile)
	if err != nil {
		log.Fatalln(err)
	} else {
		log.Debugln("read config:", config)
	}

	var buildPath string

	// stage 1
	if *f_stage2 == "" {
		// create a build path
		buildPath, err = ioutil.TempDir("", "vmbetter_build_")
		if err != nil {
			log.Fatalln("cannot create temporary directory:", err)
		}
		log.Debugln("using build path:", buildPath)

		// invoke debootstrap
		fmt.Println("invoking debootstrap (this may take a while)...")
		err = Debootstrap(buildPath, config)
		if err != nil {
			log.Fatalln(err)
		}

		// copy any overlay into place in reverse order of opened dependencies
		fmt.Println("copying overlays")
		err = Overlays(buildPath, config)
		if err != nil {
			log.Fatalln(err)
		}

		//
		// stage 1 complete
		//

		if *f_stage1 {
			stage1Target := strings.Split(filepath.Base(config.Path), ".")[0] + "_stage1"
			log.Infoln("writing stage 1 target", stage1Target)

			err = os.Mkdir(stage1Target, 0666)
			if err != nil {
				log.Fatalln(err)
			}

			cmd := exec.Command("cp", "-r", "-v", buildPath+"/.", stage1Target)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				log.Fatalln(err)
			}
			stderr, err := cmd.StderrPipe()
			if err != nil {
				log.Fatalln(err)
			}
			log.LogAll(stdout, log.INFO, "cp")
			log.LogAll(stderr, log.ERROR, "cp")

			err = cmd.Run()
			if err != nil {
				log.Fatalln(err)
			}
		}
	} else {
		buildPath = *f_stage2
	}

	// stage 2
	if *f_stage2 != "" || !*f_stage1 {
		// call post build chroot commands in reverse order as well
		fmt.Println("executing post-build commands")
		err = PostBuildCommands(buildPath, config)
		if err != nil {
			log.Fatalln(err)
		}

		// build the image file
		fmt.Println("building target files")
		if *f_qcow {
			err = Buildqcow2(buildPath, config)
		} else {
			err = BuildTargets(buildPath, config)
		}
		if err != nil {
			log.Fatalln(err)
		}
	}

	// cleanup?
	if !*f_noclean && *f_stage2 == "" {
		fmt.Println("cleaning up")
		err = os.RemoveAll(buildPath)
		if err != nil {
			log.Errorln(err)
		}
	}
	fmt.Println("done")
}

// LogSetup creates loggers on stderr or to file, based on input flags.
func LogSetup() {
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
