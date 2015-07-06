package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"minicli"
	"miniclient"
	log "minilog"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	BASE_PATH = "/tmp/minimega"
	BANNER    = `minimega, Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.`
)

var (
	f_base     = flag.String("base", BASE_PATH, "base path for minimega data")
	f_preamble = flag.String("preamble", "", "path to file containing minimega commands to run on startup")
	f_testDir  = flag.String("dir", "tests", "path to directory containing tests")
	f_loglevel = flag.String("level", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
)

func logSetup() {
	level, err := log.LevelInt(*f_loglevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	color := true
	if runtime.GOOS == "windows" {
		color = false
	}

	if *f_log {
		log.AddLogger("stdio", os.Stderr, level, color)
	}

	if *f_logfile != "" {
		err := os.MkdirAll(filepath.Dir(*f_logfile), 0755)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		logfile, err := os.OpenFile(*f_logfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		log.AddLogger("file", logfile, level, false)
	}
}

// runCommands reads and runs all the commands from a file. Return the
// concatenation of all the Responses or an error.
func runCommands(mm *miniclient.Conn, file string) (string, error) {
	var res string
	var err error

	f, err := os.Open(file)
	if err != nil {
		return "", err
	}

	s := bufio.NewScanner(f)

	for s.Scan() {
		// Can't use Compile since we didn't minimega, not minitest, registers
		// handlers with minicli
		cmd := &minicli.Command{Original: s.Text()}

		for resps := range mm.Run(cmd) {
			if err != nil {
				continue
			}

			res += fmt.Sprintf("## %v\n", cmd.Original)

			for _, resp := range resps.Resp {
				if resp.Error != "" {
					res += fmt.Sprintf("E: %v\n", resp.Error)
				}
			}

			if len(resps.Rendered) > 0 {
				res += resps.Rendered + "\n"
			}
		}
	}

	if err := s.Err(); err != nil {
		return "", err
	}

	return res, nil
}

func runTests() {
	mm, err := miniclient.Dial(*f_base)
	if err != nil {
		log.Fatal("%v", err)
	}

	if *f_preamble != "" {
		out, err := runCommands(mm, *f_preamble)
		if err != nil {
			log.Fatal("%v", err)
		}

		log.Info(out)
	}

	// TODO: Should we quit minimega and restart it between each test?
	//quit := mustCompile(t, "quit 2")

	files, err := ioutil.ReadDir(*f_testDir)
	if err != nil {
		log.Fatal("%v", err)
	}

	for _, info := range files {
		if strings.HasSuffix(info.Name(), ".want") || strings.HasSuffix(info.Name(), ".got") {
			continue
		}

		log.Info("Running commands from %s", info.Name())
		fpath := path.Join(*f_testDir, info.Name())

		got, err := runCommands(mm, fpath)
		if err != nil {
			log.Fatal("%v", err)
		}

		// Record the output for offline comparison
		if err := ioutil.WriteFile(fpath+".got", []byte(got), os.FileMode(0644)); err != nil {
			log.Error("unable to write `%s` -- %v", fpath+".got", err)
		}

		want, err := ioutil.ReadFile(fpath + ".want")
		if err != nil {
			log.Error("unable to read file `%s` -- %v", fpath+".want", err)
			continue
		}

		if got != string(want) {
			log.Error("got != want for %s", info.Name())
		}

		//mm.runCommand(quit)
	}
}

func usage() {
	fmt.Println(BANNER)
	fmt.Println("usage: minimega [option]... [file]...")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if !strings.HasSuffix(*f_base, "/") {
		*f_base += "/"
	}

	flag.Parse()

	logSetup()

	// TODO: Run minimega, and keep restarting it until all the tests have been
	// run. This allows us to not worry about cleaning up the state fully
	// within each test.

	runTests()
}
