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
	log "minilog"
	"os"
	"path/filepath"
	"strings"
	"text/scanner"
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
}

// Read config returns a Config object with the config file parameters and
// any parents. Config is invalid on any non-nil error.
func ReadConfig(path string) (c Config, err error) {
	c.Path = path
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
	tok := s.Scan()
	for tok != scanner.EOF {
		pos := s.Pos()
		if tok != scanner.Ident {
			err = fmt.Errorf("%s:%s malformed config: %s, expected identifier, got %s", path, pos, s.TokenText(), scanner.TokenString(tok))
			return err
		}
		k := s.TokenText()
		tok = s.Scan()
		if tok != '=' {
			err = fmt.Errorf("%s:%s malformed config: %s, expected '=', got %s", path, pos, s.TokenText(), scanner.TokenString(tok))
			return err
		}
		tok = s.Scan()
		if tok != scanner.String {
			err = fmt.Errorf("%s:%s malformed config %s, expected string, got %s", path, pos, s.TokenText(), scanner.TokenString(tok))
			return err
		}

		v := strings.Trim(s.TokenText(), "\"`")
		d := strings.Fields(v)
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
			err = fmt.Errorf("invalid key %s", k)
			return err
		}
		tok = s.Scan()
	}
	return nil
}
