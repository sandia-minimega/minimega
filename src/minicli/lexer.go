package minicli

import (
	"bufio"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

type ItemType int

const (
	NoType ItemType = iota
	Literal
	ReqString
	OptString
	ReqChoice
	OptChoice
	ReqList
	OptList
	CmdString
)

var terminalsToTypes = map[string]ItemType{
	">": ReqString,
	"]": OptString,
	")": CmdString,
}

var listTerminalsToTypes = map[string]ItemType{
	">": ReqList,
	"]": OptList,
}

var requireEndOfLine = []ItemType{
	OptString, OptChoice, ReqList, OptList, OptString, CmdString,
}

type PatternItem struct {
	// The item type e.g. string literal, required string
	Type ItemType
	// Key is usually the first word, so "<foo bar>"->"foo"
	Key string
	// The original full text of the token
	Text string
	// A list of the options in the case of multiple choice
	Options []string
}

type stateFn func(*patternLexer) (stateFn, error)

type patternLexer struct {
	s        *bufio.Scanner
	state    stateFn
	items    []PatternItem
	newItem  PatternItem
	terminal string
}

func (l *patternLexer) Run() (err error) {
	for state := l.state; state != nil && err == nil; {
		state, err = state(l)
	}

	return err
}

// lexOutside is our starting state. When we're in this state, we look for the
// start of an optional or required string (or list). While scanning, we
// may produce a string literal.
func lexOutside(l *patternLexer) (stateFn, error) {
	// Content scanned so far
	var content string

	for l.s.Scan() {
		token := l.s.Text()
		switch token {
		case "<":
			// Found the start of a required string (or list of strings)
			l.terminal = ">"
			return lexVariable, nil
		case "[":
			// Found the start of an optional string (or list of strings)
			l.terminal = "]"
			return lexVariable, nil
		case "(":
			// Found the start of a nested command
			l.terminal = ")"
			return lexVariable, nil
		case `"`, `'`:
			return nil, errors.New("single and double quotes are not allowed")
		default:
			// Found the end of a string literal
			r, _ := utf8.DecodeRuneInString(token)
			if unicode.IsSpace(r) {
				item := PatternItem{Type: Literal, Text: content}
				l.items = append(l.items, item)
				return lexOutside, nil
			}

			content += token
		}
	}

	// Finished parsing pattern with no errors... Yippie kay yay
	return nil, nil
}

// lexVariable is the state where we've encountered a "<", "[", or "(" and are
// scanning for the terminating ">", "]", or ")". Switches to lexMulti or
// lexComment if we find a comma or a space, respectively.
func lexVariable(l *patternLexer) (stateFn, error) {
	// Content scanned so far
	var content string

	l.newItem = PatternItem{Type: terminalsToTypes[l.terminal]}

	// Scan until EOF, checking each token
	for l.s.Scan() {
		token := l.s.Text()
		switch token {
		case ",":
			// Found a comma, should switch to lexMulti state
			l.newItem.Options = []string{content}
			content += token
			l.newItem.Text = content
			return lexMulti, nil
		case "<", "[", "(":
			// Pattern seems to be trying to use nesting which is not allowed
			return nil, errors.New("cannot nest items")
		case `"`, `'`:
			return nil, errors.New("single and double quotes are not allowed")
		case l.terminal:
			// Found terminal token, prepare to emit item
			l.newItem.Key = content
			l.newItem.Text = content

			// Check for ... if terminal != ")"
			if l.terminal == ">" || l.terminal == "]" {
				if list, err := l.checkList(); err != nil {
					return nil, err
				} else if list {
					l.newItem.Type = listTerminalsToTypes[l.terminal]
				}
			}

			// Make sure we're at EOF if we need to be
			if err := l.enforceEOF(); err != nil {
				return nil, err
			}

			// Emit Item
			l.items = append(l.items, l.newItem)
			return lexOutside, nil
		default:
			// If there's a space, we've found the end of the key
			r, _ := utf8.DecodeRuneInString(token)
			if unicode.IsSpace(r) {
				l.newItem.Key = content
				content += token
				l.newItem.Text = content
				return lexComment, nil
			}

			content += token
		}
	}

	// We must have hit EOF before changing state
	return nil, fmt.Errorf("missing terminal %s", l.terminal)
}

// lexMulti scans the pattern and figures out what multiple choice options the
// command accepts. It keeps scanning until it hits the terminal character.
func lexMulti(l *patternLexer) (stateFn, error) {
	// Content scanned so far
	var content string

	// Scan until EOF, checking each token
	for l.s.Scan() {
		token := l.s.Text()
		switch token {
		case ",":
			// Found the end of another multi-choice option
			l.newItem.Options = append(l.newItem.Options, content)
			content += token
			l.newItem.Text += content
			return lexMulti, nil
		case "<", "[", "(":
			// Pattern seems to be trying to use nesting which is not allowed
			return nil, errors.New("cannot nest items")
		case `"`, `'`:
			return nil, errors.New("single and double quotes are not allowed")
		case l.terminal:
			// Found terminal token, prepare to emit item
			l.newItem.Options = append(l.newItem.Options, content)
			l.newItem.Text += content

			switch l.terminal {
			case ">":
				l.newItem.Type = ReqChoice
			case "]":
				l.newItem.Type = OptChoice
			default:
				// Should never happen
				return nil, errors.New("something wicked happened")
			}

			// Make sure we're at EOF if we need to be
			if err := l.enforceEOF(); err != nil {
				return nil, err
			}

			// Emit Item
			l.items = append(l.items, l.newItem)
			return lexOutside, nil
		default:
			// Ensure that the current token is not whitespace
			r, _ := utf8.DecodeRuneInString(token)
			if unicode.IsSpace(r) {
				return nil, errors.New("spaces not allowed in multiple choice")
			}

			// Update content scanned so far
			content += token
		}
	}

	// We must have hit EOF before changing state
	return nil, fmt.Errorf("missing terminal %s", l.terminal)
}

// lexComment is used to consume a comment that comes after the key. Will
// consume tokens until it hits the terminal character.
func lexComment(l *patternLexer) (stateFn, error) {
	// Content scanned so far
	var content string

	// Scan until EOF, checking each token
	for l.s.Scan() {
		token := l.s.Text()
		switch token {
		case "[", "<", "(":
			// Pattern seems to be trying to use nesting which is not allowed
			return nil, errors.New("cannot nest items")
		case `"`, `'`:
			return nil, errors.New("single and double quotes are not allowed")
		case l.terminal:
			// Found terminal toke, prepare to emit item
			l.newItem.Text += content

			if list, err := l.checkList(); err != nil {
				return nil, err
			} else if list {
				l.newItem.Type = listTerminalsToTypes[l.terminal]

				// Make sure we're at EOF if we need to be
				if err := l.enforceEOF(); err != nil {
					return nil, err
				}
			}

			// Emit item
			l.items = append(l.items, l.newItem)
			return lexOutside, nil
		default:
			// Update content scanned so far
			content += token
		}
	}

	// We must have hit EOF before changing state
	return nil, fmt.Errorf("missing terminal %s", l.terminal)
}

// enforceEOF makes sure that we are at the end of the line if the item we're
// building requires it.
func (l *patternLexer) enforceEOF() error {
	if l.newItem.Type == NoType {
		panic(errors.New("cannot enforce EOF when item type not specified"))
	}

	for _, t := range requireEndOfLine {
		if l.newItem.Type == t {
			if l.s.Scan() {
				return errors.New("trailing characters when EOF expected")
			}
		}
	}

	return nil
}

// checkList checks if the remaining characters in the pattern are ...
func (l *patternLexer) checkList() (bool, error) {
	var count int

	err := fmt.Errorf("invalid trailing characters after %s", l.terminal)

	for l.s.Scan() {
		token := l.s.Text()
		r, _ := utf8.DecodeRuneInString(token)
		if unicode.IsSpace(r) {
			break
		} else if token != "." {
			return false, err
		}

		count += 1
	}

	if count != 0 && count != 3 {
		return false, err
	}

	return (count == 3), nil
}
