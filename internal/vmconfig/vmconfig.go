// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// package vmconfig reads in a config file for the vmbetter tool.
// vmconfig config files use valid Go syntax and are parsed by a go lexical
// scanner.
//
// See example.conf in the vmconfig source tree for an example.
package vmconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/scanner"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Config contains a complete vmconfig configuration and is returned by
// ReadConfig. Config also contains the vmconfig parameters, in depth-first
// order of any parents inherited by the top level config.
type Config struct {
	Path       string   // path to the head config file (passed to vmbetter)
	Parents    []string // paths to all dependent config files in order
	Packages   []string // list of in order packages to include (although order shouldn't matter)
	Overlays   []string // reverse order list of overlays
	Postbuilds []string // post build commands

	Constraints []string // constraints used when parsing file
}

// matches tests whether c matches the constraints.
func (c *Config) matches(constraints []string) bool {
outer:
	for _, v := range constraints {
		for _, v2 := range c.Constraints {
			if strings.HasPrefix(v, "!") && v[1:] == v2 {
				// explicitly matched a ! rule -- return immediately
				return false
			} else if v == v2 {
				// matched this constraint, check the next one
				continue outer
			}
		}

		// didn't match this constraint
		return false
	}

	// must have matched all constraints
	return true
}

// Read config returns a Config object with the config file parameters and any
// parents. Config is invalid on any non-nil error.
func ReadConfig(path string, constraints ...string) (c Config, err error) {
	c.Path = path
	c.Constraints = constraints
	err = read(path, "", &c)
	return
}

// reentrant read routine. Will be called recursively if a 'parents' key exists in the config file
func read(path, prev string, c *Config) error {
	f, err := os.Open(path)
	if err != nil {
		// file doesn't exist, let's try some path magic
		if strings.Contains(err.Error(), "no such file") && prev != "" {
			// maybe the parent is relative to the prev file
			path2 := filepath.Join(filepath.Dir(prev), path)

			f, err = os.Open(path2)
			if err != nil {
				return err
			}

			log.Warn("could not find %v, but found %v, using that instead", path, path2)
			path = path2
		}
	}
	defer f.Close()

	var s scanner.Scanner
	s.Init(f)
	s.Filename = path
	s.Mode ^= scanner.SkipComments // don't skip comments

	var constraints []string

	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		if tok == scanner.Comment {
			// look for +build tags
			txt := s.TokenText()
			if !strings.HasPrefix(txt, "// +build ") {
				continue
			}

			if constraints != nil {
				// that's odd, already have constraints
			}
			constraints = strings.Fields(txt)[2:]
			continue
		}

		if tok != scanner.Ident {
			return fmt.Errorf("%v malformed config: %s, expected identifier, got %s", s.Pos(), s.TokenText(), scanner.TokenString(tok))
		}
		k := s.TokenText()

		tok = s.Scan()
		if tok != '=' {
			return fmt.Errorf("%v malformed config: %s, expected '=', got %s", s.Pos(), s.TokenText(), scanner.TokenString(tok))
		}

		tok = s.Scan()
		if tok != scanner.String && tok != scanner.RawString {
			return fmt.Errorf("%v malformed config %s, expected string, got %s", s.Pos(), s.TokenText(), scanner.TokenString(tok))
		}

		v := strings.Trim(s.TokenText(), "\"`")
		d := strings.Fields(v)

		if !c.matches(constraints) {
			log.Info("%v skipping %v, does not match constraints", s.Pos(), k)
			constraints = nil
			continue
		}

		switch k {
		case "parents":
			for _, i := range d {
				log.Infoln("reading config:", i)
				err = read(i, path, c)
				c.Parents = append(c.Parents, i)
				if err != nil {
					return err
				}
			}
		case "packages":
			c.Packages = append(c.Packages, d...)
		case "overlay":
			// trim any trailing "/"
			for i, j := range d {
				d[i] = strings.TrimRight(j, "/")

				// if not absolute, the overlay should be a relative path from
				// the directory containing this config
				if !filepath.IsAbs(d[i]) {
					d[i] = filepath.Join(filepath.Dir(path), d[i])
				}
			}
			c.Overlays = append(c.Overlays, d...)
		case "postbuild":
			c.Postbuilds = append(c.Postbuilds, v)
		default:
			return fmt.Errorf("invalid key %s", k)
		}
	}
	return nil
}
