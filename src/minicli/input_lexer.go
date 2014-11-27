package minicli

import (
	"bufio"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type inputLexer struct {
	s        *bufio.Scanner
	items    []InputItem
	terminal string
}

type InputItem struct {
	Value string
	Quote string // will be `"`, `'`, or ``
}

func PrintInput(items []InputItem) string {
	parts := make([]string, len(items))
	for i, v := range items {
		parts[i] = v.Quote + v.Value + v.Quote
	}

	return strings.Join(parts, " ")
}

func (l *inputLexer) Run() (err error) {
	for state := l.lexOutside; state != nil && err == nil; {
		state, err = state()
	}

	return err
}

// lexOutside is our starting state. When we're in this state, we look for the
// start of a quote string or regular strings.
func (l *inputLexer) lexOutside() (stateFn, error) {
	// Content scanned so far
	var content string

	for l.s.Scan() {
		token := l.s.Text()
		switch token {
		case `"`, `'`:
			l.terminal = token
			return l.lexQuote, nil
		default:
			// Found the end of a string literal
			r, _ := utf8.DecodeRuneInString(token)
			if unicode.IsSpace(r) {
				l.items = append(l.items, InputItem{Value: content})
				return l.lexOutside, nil
			}

			content += token
		}
	}

	// Emit the last item on the line
	if len(content) > 0 {
		l.items = append(l.items, InputItem{Value: content})
	}

	// Finished parsing pattern with no errors... Yippie kay yay
	return nil, nil
}

// lexQuote is the state where we've encountered a " or ' and we are scanning
// for the terminating " or '.
func (l *inputLexer) lexQuote() (stateFn, error) {
	// Content scanned so far
	var content string

	// Scan until EOF, checking each token
	for l.s.Scan() {
		token := l.s.Text()
		switch token {
		case l.terminal:
			l.items = append(l.items, InputItem{Value: content, Quote: l.terminal})
			return l.lexOutside, nil
		default:
			content += token
		}
	}

	// We must have hit EOF before changing state
	return nil, fmt.Errorf("missing terminal %s", l.terminal)
}
