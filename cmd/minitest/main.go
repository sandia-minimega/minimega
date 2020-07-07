package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	BASE_PATH = "/tmp/minimega"
	BANNER    = `minimega, Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.`

	PROLOG = "prolog"
	EPILOG = "epilog"
	ENTER  = "enter"
	EXIT   = "exit"
)

var skippedExtensions = []string{
	".got",
	".want",
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
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}

	var b bytes.Buffer
	s := bufio.NewScanner(f)

	for s.Scan() {
		cmd := s.Text()

		if len(cmd) > 0 {
			fmt.Fprintf(&b, "## %v\n", cmd)
		} else {
			b.WriteString("\n")
		}

		for resps := range c.Run(cmd) {
			var errs []string
			for _, resp := range resps.Resp {
				if resp.Error != "" {
					errs = append(errs, "E: "+resp.Error)
				}
			}

			if len(errs) > 0 {
				sort.Strings(errs)
				b.WriteString(strings.Join(errs, "\n"))
				b.WriteString("\n")
			}

			if len(resps.Rendered) > 0 {
				b.WriteString(resps.Rendered)
				b.WriteString("\n")
			}
		}
	}

	if err := s.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// write s to f
func writeString(f, s string) {
	if err := ioutil.WriteFile(f, []byte(s), os.FileMode(0644)); err != nil {
		log.Error("unable to write to `%s`: %v", f, err)
	}
}

func (c Client) runTests(dir string) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal("unable to read files in %v: %v", dir, err)
	}

	// Do a quick scan of the directory to see if we can run any tests before
	// running enter
	var shouldEnter bool

	for _, info := range files {
		if info.IsDir() {
			continue
		}

		shouldEnter = shouldEnter || shouldRun(info.Name())
	}

	if !shouldEnter {
		// TODO: we could need to run tests in subdirectories... this is too
		// complicated to implement right now.
		log.Info("skipping dir, no tests to run: %v", dir)
		return
	}

	var prolog, epilog string

	// Check to see if any special files exist
	for _, info := range files {
		abs := filepath.Join(dir, info.Name())

		switch info.Name() {
		case PROLOG:
			prolog = abs
		case EPILOG:
			epilog = abs
		case ENTER:
			log.Debug("running enter dir: %v", dir)
			got := c.mustRunCommands(abs)
			writeString(abs+".got", got)
		case EXIT:
			defer func() {
				log.Debug("running exit dir: %v", dir)
				got := c.mustRunCommands(abs)
				writeString(abs+".got", got)
			}()
		}
	}

	var subdirs []string

	for _, info := range files {
		name := info.Name()
		abs := filepath.Join(dir, name)

		if info.IsDir() {
			// run the subdirectories last
			subdirs = append(subdirs, abs)
			continue
		}

		if !shouldRun(name) {
			log.Debug("skipping %v", name)
			continue
		}

		if prolog != "" {
			log.Debug("running prolog: %v", prolog)
			c.mustRunCommands(prolog)
		}

		log.Info("running commands from %s", name)
		got := c.mustRunCommands(abs)

		if epilog != "" {
			log.Debug("running epilog: %v", epilog)
			c.mustRunCommands(epilog)
		}

		// Record the output for offline comparison
		writeString(abs+".got", got)

		want, err := ioutil.ReadFile(abs + ".want")
		if err != nil {
			log.Error("unable to read file `%s` -- %v", abs+".want", err)
			continue
		}

		if got != string(want) {
			fmt.Printf("got != want for %s\n", name)
		}
	}

	for _, subdir := range subdirs {
		log.Info("processing tests in subdir: %v", subdir)
		c.runTests(subdir)
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

	// Don't run special files
	if f == PROLOG || f == EPILOG || f == ENTER || f == EXIT {
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

	c.runTests(*f_tests)
}
