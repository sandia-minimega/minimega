// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	"encoding/json"
	"fmt"
	log "minilog"
	"strings"
	"sync"
)

// Output modes
const (
	defaultMode = iota
	jsonMode
	csvMode
)

const (
	CommentLeader = "#"
)

type Flags struct {
	Annotate   bool
	Compress   bool
	Headers    bool
	Sort       bool
	Preprocess bool
	Mode       int
	Record     bool
}

var flagsLock sync.Mutex

var (
	aliases     = map[string]string{}
	aliasesLock sync.Mutex
)

var defaultFlags = Flags{
	// Output flags
	Annotate:   true,
	Compress:   true,
	Headers:    true,
	Sort:       true,
	Preprocess: true,
	Mode:       defaultMode,

	// Command flags
	Record: true,
}

var handlers []*Handler
var history []string // command history for the write command

// HistoryLen is the length of the history of commands that minicli stores.
// This may be increased or decreased as needed. If set to 0 or less, the
// history will grow unbounded and may cause an OOM crash.
var HistoryLen = 10000

// firstHistoryTruncate stores a flag so that we can warn the user the first
// time that we're truncating history.
var firstHistoryTruncate = true

type Responses []*Response

// A response as populated by handler functions.
type Response struct {
	Host     string      // Host this response was created on
	Response string      // Simple response
	Header   []string    // Optional header. If set, will be used for both Response and Tabular data.
	Tabular  [][]string  // Optional tabular data. If set, Response will be ignored
	Error    string      // Because you can't gob/json encode an error type
	Data     interface{} `json:"-"` // Optional user data

	// Embedded output flags, overrides defaults if set for first response
	*Flags `json:"-"`
}

type CLIFunc func(*Command, chan<- Responses)
type SuggestFunc func(string, string, string) []string

// Preprocessor may be set to perform actions immediately before commands run.
var Preprocessor func(*Command) error

// Reset minicli state including all registered handlers.
func Reset() {
	handlers = nil
	history = nil
	firstHistoryTruncate = true
}

// MustRegister calls Register for a handler and panics if the handler has an
// error registering.
func MustRegister(h *Handler) {
	if err := Register(h); err != nil {
		panic(err)
	}
}

// Register a new API based on pattern. See package documentation for details
// about supported patterns.
func Register(h *Handler) error {
	if err := h.parsePatterns(); err != nil {
		return err
	}

	h.HelpShort = strings.TrimSpace(h.HelpShort)
	h.HelpLong = strings.TrimSpace(h.HelpLong)
	h.SharedPrefix = h.findPrefix()

	handlers = append(handlers, h)

	return nil
}

// Process raw input text. An error is returned if parsing the input text
// failed.
func ProcessString(input string, record bool) (<-chan Responses, error) {
	c, err := Compile(input)
	if err != nil {
		return nil, err
	}

	if c == nil {
		// Language spec: "Receiving from a nil channel blocks forever."
		// Instead, make and immediately close the channel so that range
		// doesn't block and receives no values.
		out := make(chan Responses)
		close(out)

		return out, nil
	}

	c.Record = record

	return ProcessCommand(c), nil
}

// Process a prepopulated Command
func ProcessCommand(c *Command) <-chan Responses {
	if !c.noOp && c.Call == nil {
		log.Fatal("command %v has no callback!", c)
	}

	respChan := make(chan Responses)

	go func() {
		defer close(respChan)

		// Run the preprocessor first if one is set
		if Preprocessor != nil && c.Preprocess {
			if err := Preprocessor(c); err != nil {
				resp := &Response{Error: err.Error()}
				respChan <- Responses{resp}
				return
			}
		}

		if !c.noOp {
			c.Call(c, respChan)
		}

		// Append the command to the history
		if c.Record {
			history = append(history, c.Original)

			if len(history) > HistoryLen && HistoryLen > 0 {
				if firstHistoryTruncate {
					log.Warn("history length exceeds limit, truncating to %v entries", HistoryLen)
					firstHistoryTruncate = false
				}

				history = history[len(history)-HistoryLen:]
			}
		}
	}()

	return respChan
}

// MustCompile compiles the string, calling log.Fatal if the string is not a
// valid command. Should be used when providing a known command rather than
// processing user input.
func MustCompile(input string) *Command {
	c, err := Compile(input)
	if err != nil {
		log.Fatalln(err)
	}

	return c
}

// MustCompilef wraps fmt.Sprintf and MustCompile
func MustCompilef(format string, args ...interface{}) *Command {
	return MustCompile(fmt.Sprintf(format, args...))
}

// Create a command from raw input text. An error is returned if parsing the
// input text failed.
func Compile(input string) (*Command, error) {
	input = strings.TrimSpace(input)
	if len(input) == 0 {
		return nil, nil
	}

	input = expandAliases(input)

	in, err := lexInput(input)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(input, CommentLeader) {
		cmd := &Command{Original: input, noOp: true}
		return cmd, nil
	}

	_, cmd := closestMatch(in)
	if cmd != nil {
		flagsLock.Lock()
		defer flagsLock.Unlock()

		cmd.Record = defaultFlags.Record
		cmd.Preprocess = defaultFlags.Preprocess
		return cmd, nil
	}

	return nil, fmt.Errorf("invalid command: `%s`", input)
}

// Compilef wraps fmt.Sprintf and Compile
func Compilef(format string, args ...interface{}) (*Command, error) {
	return Compile(fmt.Sprintf(format, args...))
}

// expandAliases finds the first alias match in input and replaces it with it's expansion.
func expandAliases(input string) string {
	aliasesLock.Lock()
	defer aliasesLock.Unlock()

	for k, v := range aliases {
		if strings.HasPrefix(input, k) {
			log.Info("expanding %v -> %v", k, v)
			return strings.Replace(input, k, v, 1)
		}
	}

	return input
}

func suggest(raw string, input *Input) []string {
	vals := map[string]bool{}
	for _, h := range handlers {
		for _, v := range h.suggest(raw, input) {
			vals[v] = true
		}
	}

	res := []string{}
	for k := range vals {
		res = append(res, k)
	}

	return res
}

func Suggest(input string) []string {
	log.Debug("Suggest: `%v`", input)
	in, err := lexInput(input)
	if err != nil {
		return nil
	}

	return suggest(input, in)
}

//
func Help(input string) string {
	helpShort := make(map[string]string)

	_, err := lexInput(input)
	if err != nil {
		return "Error parsing help input: " + err.Error()
	}

	// Figure out the literal string prefixes for each handler
	groups := make(map[string][]*Handler)
	for _, handler := range handlers {
		prefix := handler.SharedPrefix
		if _, ok := groups[prefix]; !ok {
			groups[prefix] = make([]*Handler, 0)
		}

		groups[prefix] = append(groups[prefix], handler)
	}

	// User entered a valid command prefix as the argument to help, display help
	// for that group of handlers.
	if group, ok := groups[input]; input != "" && ok {
		// Only one handler with a given pattern prefix, give the long help message
		if len(group) == 1 {
			return group[0].helpLong()
		}

		count := 0
		for _, v := range group {
			if len(v.HelpLong) > 0 {
				count += 1
			}
		}
		// If only one entry has long help, do magic!
		if count == 1 {
			handler := &Handler{}
			for _, v := range group {
				handler.Patterns = append(handler.Patterns, v.Patterns...)
				if len(v.HelpLong) > 0 {
					handler.HelpLong = v.HelpLong
				}
			}
			handler.parsePatterns()
			return handler.helpLong()
		}

		// Weird case, multiple handlers share the same prefix. Print the short
		// help for each handler for each pattern registered.
		// TODO: Is there something better we can do?
		for _, handler := range group {
			for _, pattern := range handler.Patterns {
				helpShort[pattern] = handler.helpShort()
			}
		}

		return printHelpShort(helpShort)
	}

	// Look for groups who have input as a prefix of the prefix, print help for
	// the handlers in those groups. If input is the empty string, we will end
	// up printing the full help short.
	matches := []string{}
	for prefix := range groups {
		if strings.HasPrefix(prefix, input) {
			matches = append(matches, prefix)
		}
	}

	if len(matches) == 0 {
		// If there's a closest match, display the long help for it
		//handler, _ := closestMatch(inputItems)
		//if handler != nil {
		//	return handler.helpLong()
		//}

		// Found an unresolvable command
		return fmt.Sprintf("no help entry for `%s`", input)
	} else if len(matches) == 1 && len(groups[matches[0]]) == 1 {
		// Very special case, one prefix match and only one handler.
		return groups[matches[0]][0].helpLong()
	}

	// List help short for all matches
	for _, prefix := range matches {
		group := groups[prefix]
		if len(group) == 1 {
			helpShort[prefix] = group[0].helpShort()
		} else {
			for _, handler := range group {
				for _, pattern := range handler.Patterns {
					helpShort[pattern] = handler.helpShort()
				}
			}
		}
	}

	return printHelpShort(helpShort)
}

func (c Command) String() string {
	return c.Original
}

// Return a verbose output representation for use with the %#v verb in pkg fmt
func (r Responses) GoString() {

}

// History returns a newline-separated string of all the commands that have
// been run by minicli since it started or the last time that ClearHistory was
// called.
func History() string {
	return strings.Join(history, "\n")
}

// ClearHistory clears the command history.
func ClearHistory() {
	history = make([]string, 0)
}

// Doc generate CLI documentation, in JSON format.
func Doc() (string, error) {
	bytes, err := json.Marshal(handlers)
	return string(bytes), err
}

// copyFlags returns a copy of the default flags
func copyFlags() *Flags {
	flagsLock.Lock()
	defer flagsLock.Unlock()

	res := &Flags{}
	*res = defaultFlags
	return res
}
