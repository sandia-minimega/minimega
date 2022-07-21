// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	f_keys         = flag.String("keys", "", "authorized_keys formatted file to install for root")
	f_passwordless = flag.Bool("passwordless", true, "True if image should contain its own id_rsa.pub in authorized_keys for passwordless ssh between nodes")
)

func usage() {
	fmt.Println("usage: passwordify [option]... [source initramfs] [destination initramfs]")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	log.Init()

	if flag.NArg() != 2 {
		usage()
		os.Exit(1)
	}

	externalCheck()

	source := flag.Arg(0)
	destination := flag.Arg(1)

	// Make sure the source exists
	if _, err := os.Stat(source); os.IsNotExist(err) {
		log.Fatalln("cannot find source initramfs", source, ":", err)
	}

	// Working directory
	tdir, err := ioutil.TempDir("", "passwordify")
	if err != nil {
		log.Fatalln("Cannot create tempdir:", err)
	}

	// Unpack initrd
	initrdCommand := fmt.Sprintf("cd %v && zcat %v | cpio -idmv", tdir, source)
	cmd := exec.Command("bash", "-c", initrdCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalln(err)
	}
	log.Info("repack: %v\n", output)

	// Set password
	p := process("chroot")
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			tdir,
			"passwd",
		},
		Dir: "",
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalln(err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalln(err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalln(err)
	}

	go func() {
		defer stdin.Close()
		var pw1, pw2 []byte
		for {
			fmt.Printf("Enter new root password: ")
			pw1, err = terminal.ReadPassword(int(syscall.Stdin))
			if err != nil {
				log.Fatalln(err)
			}
			fmt.Printf("\nRetype new root password: ")
			pw2, err = terminal.ReadPassword(int(syscall.Stdin))
			if err != nil {
				log.Fatalln(err)
			}
			fmt.Printf("\n")
			if string(pw1) != string(pw2) {
				log.Errorln("passwords do not match")
			} else if string(pw1) == "" {
				log.Errorln("password must not be blank")
			} else {
				break
			}
		}
		io.WriteString(stdin, string(pw1)+"\n")
		io.WriteString(stdin, string(pw1)+"\n")
	}()

	log.LogAll(stdout, log.INFO, "chroot")
	log.LogAll(stderr, log.INFO, "chroot")

	cmd.Run()

	// If keyfile, copy keyfile
	if *f_keys != "" {
		in, err := os.Open(*f_keys)
		if err != nil {
			log.Fatalln("can't open key file source:", err)
		}
		defer in.Close()

		err = os.Mkdir(filepath.Join(tdir, "root/.ssh"), os.ModeDir|0700)
		if err != nil {
			log.Fatalln("can't make root's ssh directory:", err)
		}

		out, err := os.OpenFile(filepath.Join(tdir, "root/.ssh/authorized_keys"), os.O_RDWR|os.O_CREATE, 0600)
		if err != nil {
			log.Fatalln("Can't open authorized_keys file:", err)
		}
		defer out.Close()

		if _, err = io.Copy(out, in); err != nil {
			log.Fatalln("Couldn't copy to authorized_keys:", err)
		}
		out.Sync()
	}

	// Generate an ssh key and append it to authorized_keys for
	// passwordless login between hosts running this initrd.
	if *f_passwordless {
		cmd = exec.Command("ssh-keygen", "-f", filepath.Join(tdir, "root/.ssh/id_rsa"), "-N", "")
		stdoutStderr, err := cmd.CombinedOutput()
		if err != nil {
			log.Fatalln(err)
		}
		log.Info("ssh-keygen: %s\n", stdoutStderr)

		// Open the authorized keys file...
		f, err := os.OpenFile(filepath.Join(tdir, "root/.ssh/authorized_keys"), os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatalln(err)
		}

		defer f.Close()
		// Read in the newly generated key...
		key, err := ioutil.ReadFile(filepath.Join(tdir, "root/.ssh/id_rsa.pub"))
		if err != nil {
			log.Fatalln(err)
		}

		// And copy it over
		if _, err = f.Write(key); err != nil {
			log.Fatalln(err)
		}
	}

	// Repack initrd
	initrdCommand = fmt.Sprintf("cd %v && find . -print0 | cpio --quiet  --null -ov --format=newc | gzip -9 > %v", tdir, destination)
	cmd = exec.Command("bash", "-c", initrdCommand)
	output, err = cmd.CombinedOutput()
	if err != nil {
		log.Fatalln(err)
	}
	log.Info("repack: %v\n", output)

	// Cleanup
	err = os.RemoveAll(tdir)
	if err != nil {
		log.Fatalln(err)
	}
}
