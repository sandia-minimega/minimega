// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// This is based on https://golang.org/src/text/template/parse/lex.go

package main

import (
	"errors"
	log "minilog"
	"strconv"
	"strings"
	"unicode/utf8"
)

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*tagLexer) stateFn

// lexer holds the state of the scanner.
type tagLexer struct {
	input   string      // the string being scanned
	state   stateFn     // the next lexing function to enter
	pos     int         // current position in the input
	start   int         // start position of this item
	width   int         // width of last rune read from input
	lastPos int         // position of most recent item returned by nextItem
	items   chan string // channel of scanned strings
	err     error
}

const eof = -1

// next returns the next rune in the input.
func (l *tagLexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = w
	l.pos += l.width
	return r
}

// emit passes an item back to the client.
func (l *tagLexer) emit() {
	l.items <- l.input[l.start:l.pos]
	l.start = l.pos
}

// run runs the state machine for the tagLexer.
func (l *tagLexer) run() {
	for l.state = lexOutside; l.state != nil; {
		l.state = l.state(l)
	}
	close(l.items)
}

// lexOutside scans for the next quoted item
func lexOutside(l *tagLexer) stateFn {
	// Throw away characters until we hit EOF or quotes
	switch r := l.next(); {
	case r == eof:
		return nil
	case r == '"':
		l.start = l.pos - l.width
		return lexQuote
	}
	return lexOutside
}

// lexQuote scans for the end of the quoted item
func lexQuote(l *tagLexer) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof:
			l.err = errors.New("unterminated quoted string")
			return nil
		case '"':
			break Loop
		}
	}
	l.emit()
	return lexOutside
}

// ParseVmTags is used to parse the %q-formatted Tags sent in a
// minicli.Response back into a map.
func ParseVmTags(s string) (map[string]string, error) {
	if !strings.HasPrefix(s, "map[") || !strings.HasSuffix(s, "]") {
		return nil, errors.New("expected `map[...]`")
	}

	tags := map[string]string{}

	// Trim off `map[` and `]`
	l := tagLexer{
		input: s,
		items: make(chan string),
	}
	go l.run()

	var key string
	for v := range l.items {
		v, err := strconv.Unquote(v)
		if err != nil {
			log.Error("invalid tags -- %v", err)
			break
		}

		if key == "" {
			key = v
		} else {
			tags[key] = v
			key = ""
		}
	}

	for range l.items {
		// Do nothing, loop consumes remaining items after an error
	}

	if l.err != nil {
		return nil, l.err
	}

	return tags, nil
}
