// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package minicli

import (
	"bufio"
	"errors"
	"fmt"
	log "minilog"
	"strings"
	"unicode"
	"unicode/utf8"
)

type itemType int

const (
	optionalItem itemType = 1 << iota
	literalItem
	commandItem
	stringItem
	choiceItem
	listItem
)

var terminalsToTypes = map[string]itemType{
	">": stringItem,
	"]": stringItem | optionalItem,
	")": commandItem,
}

var requireEOLItems = listItem | commandItem | optionalItem

type PatternItem struct {
	// The item type e.g. string literal, required string
	Type itemType `json:"type"`
	// Key is usually the first word, so "<foo bar>"->"foo"
	Key string `json:"key,omitempty"`
	// The original full text of the token
	Text string `json:"text,omitempty"`
	// A list of the options in the case of multiple choice
	Options []string `json:"options,omitempty"`
}

type PatternItems []PatternItem

func (p PatternItem) IsOptional() bool {
	return p.Type&optionalItem != 0
}

func (p PatternItem) IsLiteral() bool {
	return p.Type&literalItem != 0
}

func (p PatternItem) IsCommand() bool {
	return p.Type&commandItem != 0
}

func (p PatternItem) IsString() bool {
	return p.Type&stringItem != 0
}

func (p PatternItem) IsChoice() bool {
	return p.Type&choiceItem != 0
}

func (p PatternItem) IsList() bool {
	return p.Type&listItem != 0
}

func (items PatternItems) String() string {
	parts := make([]string, len(items))

	for i, v := range items {
		var prefix, text, suffix string
		text = v.Text

		switch v.Type {
		case literalItem:
			// Nada
		case stringItem, choiceItem:
			// Special case, required choice with one option which collapses to
			// just a required string (with some extra semantics to help in the
			// CLI handler).
			if len(v.Options) == 1 {
				text = v.Options[0]
			} else {
				prefix, suffix = "<", ">"
			}
		case stringItem | optionalItem, choiceItem | optionalItem:
			prefix, suffix = "[", "]"
		case listItem:
			prefix, suffix = "<", ">..."
		case listItem | optionalItem:
			prefix, suffix = "[", "]..."
		case commandItem:
			prefix, suffix = "(", ")"
		}

		parts[i] = prefix + text + suffix
	}

	return strings.Join(parts, " ")
}

type stateFn func() (stateFn, error)

type patternLexer struct {
	s        *bufio.Scanner
	items    []PatternItem
	newItem  PatternItem
	terminal string
}

func lexPattern(pattern string) ([]PatternItem, error) {
	s := bufio.NewScanner(strings.NewReader(pattern))
	s.Split(bufio.ScanRunes)
	l := patternLexer{s: s, items: make([]PatternItem, 0)}

	if err := l.Run(); err != nil {
		return nil, err
	}

	return l.items, nil
}

func (l *patternLexer) Run() (err error) {
	for state := l.lexOutside; state != nil && err == nil; {
		state, err = state()
	}

	return err
}

// lexOutside is our starting state. When we're in this state, we look for the
// start of an optional or required string (or list). While scanning, we
// may produce a string literal.
func (l *patternLexer) lexOutside() (fn stateFn, err error) {
	// Content scanned so far
	var content string

	defer func() {
		if err == nil && len(content) > 0 {
			item := PatternItem{Type: literalItem, Text: content}
			l.items = append(l.items, item)
		}
	}()

	for l.s.Scan() {
		token := l.s.Text()
		switch token {
		case "<":
			// Found the start of a required string (or list of strings)
			l.terminal = ">"
			return l.lexVariable, nil
		case "[":
			// Found the start of an optional string (or list of strings)
			l.terminal = "]"
			return l.lexVariable, nil
		case "(":
			// Found the start of a nested command
			l.terminal = ")"
			return l.lexVariable, nil
		case `"`, `'`:
			return nil, errors.New("single and double quotes are not allowed")
		default:
			// Found the end of a string literal
			r, _ := utf8.DecodeRuneInString(token)
			if unicode.IsSpace(r) {
				return l.lexOutside, nil
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
func (l *patternLexer) lexVariable() (stateFn, error) {
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
			return l.lexMulti, nil
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
					l.newItem.Type = listItem
					if l.terminal == "]" {
						l.newItem.Type |= optionalItem
					}
				}
			}

			// Make sure we're at EOF if we need to be
			if err := l.enforceEOF(); err != nil {
				return nil, err
			}

			// Emit Item
			l.items = append(l.items, l.newItem)
			return l.lexOutside, nil
		default:
			// If there's a space, we've found the end of the key
			r, _ := utf8.DecodeRuneInString(token)
			if unicode.IsSpace(r) {
				l.newItem.Key = content
				content += token
				l.newItem.Text = content
				return l.lexComment, nil
			}

			content += token
		}
	}

	// We must have hit EOF before changing state
	return nil, fmt.Errorf("missing terminal %s", l.terminal)
}

// lexMulti scans the pattern and figures out what multiple choice options the
// command accepts. It keeps scanning until it hits the terminal character.
func (l *patternLexer) lexMulti() (stateFn, error) {
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
			return l.lexMulti, nil
		case "<", "[", "(":
			// Pattern seems to be trying to use nesting which is not allowed
			return nil, errors.New("cannot nest items")
		case `"`, `'`:
			return nil, errors.New("single and double quotes are not allowed")
		case l.terminal:
			if len(content) > 0 {
				// Found terminal token, prepare to emit item
				l.newItem.Options = append(l.newItem.Options, content)
				l.newItem.Text += content
			}

			l.newItem.Type = choiceItem
			if l.terminal == "]" {
				l.newItem.Type |= optionalItem
			}

			// Make sure we're at EOF if we need to be
			if err := l.enforceEOF(); err != nil {
				return nil, err
			}

			// Emit Item
			l.items = append(l.items, l.newItem)
			return l.lexOutside, nil
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
func (l *patternLexer) lexComment() (stateFn, error) {
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
				l.newItem.Type = listItem
				if l.terminal == "]" {
					l.newItem.Type |= optionalItem
				}

				// Make sure we're at EOF if we need to be
				if err := l.enforceEOF(); err != nil {
					return nil, err
				}
			}

			// Emit item
			l.items = append(l.items, l.newItem)
			return l.lexOutside, nil
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
	if l.newItem.Type == 0 {
		log.Fatalln("cannot enforce EOF when item type not specified")
	}

	if l.newItem.Type&requireEOLItems != 0 {
		if l.s.Scan() {
			return errors.New("trailing characters when EOF expected")
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
