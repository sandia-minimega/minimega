package vmconfig

import (
	"text/scanner"
	"os"
	"fmt"
	"strings"
	log "minilog"
)

type Config struct {
	Path string // path to the head config file (passed to vmbetter)
	Parents []string // paths to all dependent config files in order
	Packages []string // list of in order packages to include (although order shouldn't matter)
	Overlays []string // reverse order list of overlays
}

func ReadConfig(path string) (c Config, err error) {
	c.Path = path
	err = read(path, &c)
	return
}

// reentrant read routine. Will be called recursively if a 'parents' key exists in the config file
func read(path string, c *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
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
		if tok !=  '=' {
			err = fmt.Errorf("%s:%s malformed config: %s, expected '=', got %s", path, pos, s.TokenText(), scanner.TokenString(tok))
			return err
		}
		tok = s.Scan()
		if tok != scanner.String {
			err = fmt.Errorf("%s:%s malformed config %s, expected string, got %s", path, pos, s.TokenText(), scanner.TokenString(tok))
			return err
		}
		v := strings.Trim(s.TokenText(), "\"")
		d := strings.Fields(v)
		switch k {
		case "parents":
			c.Parents = append(c.Parents, d...)
			for _, i := range d {
				log.Infoln("reading config:", i)
				err = read(i, c)
				if err != nil {
					return err
				}
			}
		case "packages":
			c.Packages = append(c.Packages, d...)
		case "overlay":
			c.Overlays = append(c.Overlays, d...)
		default:
			err = fmt.Errorf("invalid key %s", k, d)
			return err
		}
		tok = s.Scan()
	}
	return nil
}

