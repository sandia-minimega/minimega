package vmconfig

import (
	"text/scanner"
	"os"
	"fmt"
	"strings"
)

func ReadConfig(path string) (config map[string]string, err error) {
	config = make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	var s scanner.Scanner
	s.Init(f)
	tok := s.Scan()
	for tok != scanner.EOF {
		pos := s.Pos()
		if tok != scanner.Ident {
			err = fmt.Errorf("%s:%s malformed config: %s, expected identifier, got %s", path, pos, s.TokenText(), scanner.TokenString(tok))
			return
		}
		k := s.TokenText()
		tok = s.Scan()
		if tok !=  '=' {
			err = fmt.Errorf("%s:%s malformed config: %s, expected '=', got %s", path, pos, s.TokenText(), scanner.TokenString(tok))
			return
		}
		tok = s.Scan()
		if tok != scanner.String {
			err = fmt.Errorf("%s:%s malformed config %s, expected string, got %s", path, pos, s.TokenText(), scanner.TokenString(tok))
			return
		}
		v := s.TokenText()
		config[k] = strings.Trim(v,"\"")
		tok = s.Scan()
	}
	return
}

