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
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sandia-minimega/minimega/v2/internal/vmconfig"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	f_debian_mirror = flag.String("mirror", "http://ftp.us.debian.org/debian", "path to the debian mirror to use")
	f_noclean       = flag.Bool("noclean", false, "do not remove build directory")
	f_stage1        = flag.Bool("1", false, "stop after stage one, and copy build files to <config>_stage1")
	f_stage2        = flag.String("2", "", "complete stage 2 from an existing stage 1 directory")
	f_branch        = flag.String("branch", "testing", "debian branch to use")
	f_disk          = flag.Bool("disk", false, "generate a disk image, use -format to set format")
	f_diskSize      = flag.String("size", "1G", "disk image size (e.g. 1G, 1024M)")
	f_format        = flag.String("format", "qcow2", "disk format to use when -disk is set")
	f_mbr           = flag.String("mbr", "/usr/lib/syslinux/mbr/mbr.bin", "path to mbr.bin if building disk images")
	f_iso           = flag.Bool("iso", false, "generate an ISO")
	f_isolinux      = flag.String("isolinux", "misc/isolinux/", "path to a directory containing isolinux.bin, ldlinux.c32, and isolinux.cfg")
	f_rootfs        = flag.Bool("rootfs", false, "generate a simple rootfs")
	f_dstrp_append  = flag.String("debootstrap-append", "", "additional arguments to be passed to debootstrap")
	f_constraints   = flag.String("constraints", "debian,amd64", "specify build constraints, separated by commas")
	f_target        = flag.String("O", "", "specify output name, by default uses name of config")
	f_dry_run       = flag.Bool("dry-run", false, "parse and print configs and then exit")
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

func main() {
	flag.Usage = usage
	flag.Parse()

	log.Init()

	if flag.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	externalCheck()

	// stage 1 and stage 2 flags are mutually exclusive
	if *f_stage1 && *f_stage2 != "" {
		log.Fatalln("-1 cannot be used with -2")
	}

	// find any other dependent configs and get an ordered list of those
	configfile := flag.Arg(0)
	log.Debugln("using config:", configfile)
	config, err := vmconfig.ReadConfig(configfile, strings.Split(*f_constraints, ",")...)
	if err != nil {
		log.Fatalln(err)
	} else if *f_dry_run {
		fmt.Printf("%v", config)
		os.Exit(0)
	} else {
		log.Debugln("read config:", config)
	}

	// If we're doing a LiveCD, we need to add the live-boot package
	if *f_iso {
		config.Packages = append(config.Packages, "live-boot")
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

			p := process("cp")
			cmd := exec.Command(p, "-r", "-v", buildPath+"/.", stage1Target)
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
		if *f_disk {
			err = BuildDisk(buildPath, config)
		} else if *f_iso {
			err = BuildISO(buildPath, config)
		} else if *f_rootfs {
			err = BuildRootFS(buildPath, config)
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
