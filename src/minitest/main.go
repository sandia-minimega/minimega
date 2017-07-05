package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"miniclient"
	log "minilog"
	"os"
	"path"
	"regexp"
	"strings"
)

const (
	BASE_PATH = "/tmp/minimega"
	BANNER    = `minimega, Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.`

	PROLOG = "prolog"
	EPILOG = "epilog"
)

var skippedExtensions = []string{
	"got",
	"want",
}

var (
	f_base  = flag.String("base", BASE_PATH, "base path for minimega data")
	f_tests = flag.String("dir", "tests", "path to directory containing tests")
	f_run   = flag.String("run", "", "run only tests matching the regular expression")
)

// matchRe is compiled from f_run before calling runTests
var matchRe *regexp.Regexp

type Client struct {
	*miniclient.Conn // embed
}

// mustRunCommands wraps runCommands and calls log.Fatal if there's an error.
func (c Client) mustRunCommands(file string) string {
	s, err := c.runCommands(file)
	if err != nil {
		log.Fatal("%v", err)
	}

	return s
}

// runCommands reads and runs all the commands from a file. Return the
// concatenation of all the Responses or an error.
func (c Client) runCommands(file string) (string, error) {
	var res string
	var err error

	f, err := os.Open(file)
	if err != nil {
		return "", err
	}

	s := bufio.NewScanner(f)

	for s.Scan() {
		cmd := s.Text()

		if len(cmd) > 0 {
			res += fmt.Sprintf("## %v\n", cmd)
		} else {
			res += "\n"
		}

		for resps := range c.Run(cmd) {
			if err != nil {
				continue
			}

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

func (c Client) runTests(dir string, prolog, epilog []string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal("unable to read files in %v: %v", dir, err)
	}

	// Check to see if the prolog and epilog files exist
	for _, info := range files {
		name := info.Name()

		if name == PROLOG {
			// append new prolog so that it gets run last
			prolog = append(prolog, path.Join(dir, name))
		}

		if name == EPILOG {
			// prepend new epilog so that it gets run first
			epilog = append([]string{path.Join(dir, name)}, epilog...)
		}
	}

	for _, info := range files {
		name := info.Name()
		abs := path.Join(dir, name)

		if info.IsDir() {
			log.Info("processing tests on subdir: %v", name)
			c.runTests(abs, prolog, epilog)
			continue
		}

		if !shouldRun(name) {
			log.Debug("skipping %v", name)
			continue
		}

		for _, p := range prolog {
			// Run the prolog commands
			log.Debug("running prolog: %v", p)
			c.mustRunCommands(p)
		}

		log.Info("running commands from %s", name)
		got := c.mustRunCommands(abs)

		for _, e := range epilog {
			// Run the epilog commands
			log.Debug("running epilog: %v", e)
			c.mustRunCommands(e)
		}

		// Record the output for offline comparison
		if err := ioutil.WriteFile(abs+".got", []byte(got), os.FileMode(0644)); err != nil {
			log.Error("unable to write `%s` -- %v", abs+".got", err)
		}

		want, err := ioutil.ReadFile(abs + ".want")
		if err != nil {
			log.Error("unable to read file `%s` -- %v", abs+".want", err)
			continue
		}

		if got != string(want) {
			log.Error("got != want for %s", name)
		}
	}
}

// shouldRun tests to see whether a given filename represents a valid test file
func shouldRun(f string) bool {
	for _, ext := range skippedExtensions {
		if strings.HasSuffix(f, ext) {
			return false
		}
	}

	// Skip hidden files -- probably not valid tests
	if strings.HasPrefix(f, ".") {
		return false
	}

	// Don't run the prolog or epilog
	if f == PROLOG || f == EPILOG {
		return false
	}

	// If a regexp is defined, skip files that don't match
	if matchRe != nil && !matchRe.Match([]byte(f)) {
		return false
	}

	return true
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

	log.Init()

	if *f_run != "" {
		var err error
		log.Debug("only running files matching `%v`", *f_run)

		matchRe, err = regexp.Compile(*f_run)
		if err != nil {
			log.Fatal("invalid regexp: %v", err)
		}
	}

	mm, err := miniclient.Dial(*f_base)
	if err != nil {
		log.Fatal("%v", err)
	}

	c := Client{mm}

	c.runTests(*f_tests, nil, nil)
}
