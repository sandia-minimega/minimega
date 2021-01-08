// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minicli

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type inputLexer struct {
	s        *bufio.Scanner
	items    []inputItem
	terminal string
	content  string

	emit bool // force emit, even if content is empty (e.g. "" as input)

	prevState stateFn
}

type inputItem struct {
	Value string
}

type inputItems []inputItem

type Input struct {
	Original string
	items    inputItems
}

var escapedChars = map[string]string{
	"r":           "\r",
	"n":           "\n",
	"t":           "\t",
	`\`:           `\`,
	`"`:           `"`,
	`'`:           `'`,
	CommentLeader: CommentLeader,
}

func (items inputItems) String() string {
	s2 := make([]string, len(items))

	for i, v := range items {
		if strings.IndexFunc(v.Value, unicode.IsSpace) > -1 {
			s2[i] = strconv.Quote(v.Value)
		} else {
			s2[i] = v.Value
		}
	}

	return strings.Join(s2, " ")
}

func lexInput(input string) (*Input, error) {
	s := bufio.NewScanner(strings.NewReader(input))
	s.Split(bufio.ScanRunes)
	l := inputLexer{s: s, items: make([]inputItem, 0)}

	if err := l.Run(); err != nil {
		return nil, err
	}

	return &Input{Original: input, items: l.items}, nil
}

func (l *inputLexer) Run() (err error) {
	for state := l.lexOutside; state != nil && err == nil; {
		curr := state
		state, err = curr()
		l.prevState = curr
	}

	return err
}

// lexOutside is our starting state. When we're in this state, we look for the
// start of a quote string or regular strings.
func (l *inputLexer) lexOutside() (stateFn, error) {
	emitContent := func() {
		// Emit item from processed content, if non-empty
		if len(l.content) > 0 || l.emit {
			l.items = append(l.items, inputItem{Value: l.content})
			l.content = ""
			l.emit = false
		}
	}

outer:
	for l.s.Scan() {
		switch token := l.s.Text(); token {
		case `\`:
			return l.lexEscape, nil
		case `"`, `'`:
			l.terminal = token
			return l.lexQuote, nil
		case CommentLeader:
			break outer
		default:
			// Found the end of a string literal
			r, _ := utf8.DecodeRuneInString(token)
			if unicode.IsSpace(r) {
				emitContent()
				return l.lexOutside, nil
			}

			l.content += token
		}
	}

	emitContent()

	// Finished parsing pattern with no errors... Yippie kay yay
	return nil, nil
}

// lexQuote is the state where we've encountered a " or ' and we are scanning
// for the terminating " or '.
func (l *inputLexer) lexQuote() (stateFn, error) {
	// Scan until EOF, checking each token
	for l.s.Scan() {
		switch token := l.s.Text(); token {
		case `\`:
			return l.lexEscape, nil
		case l.terminal:
			l.emit = true
			return l.lexOutside, nil
		default:
			l.content += token
		}
	}

	// We must have hit EOF before changing state
	return nil, fmt.Errorf("missing terminal %s", l.terminal)
}

func (l *inputLexer) lexEscape() (stateFn, error) {
	// Must scan one character
	if !l.s.Scan() {
		return nil, errors.New("expected escape character")
	}

	token := l.s.Text()
	if v, ok := escapedChars[token]; ok {
		l.content += v
		return l.prevState, nil
	}

	return nil, fmt.Errorf("unexpected escaped character: %v", token)
}
