// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minicli

import (
	"encoding/json"
	"fmt"
	log "minilog"
	"strings"
	"sync"
	"unicode"
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
var trie = &patternTrie{
	Children: make(map[patternTrieKey]*patternTrie),
}

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
	Data     interface{} //`json:"-"` // Optional user data

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

	trie = &patternTrie{
		Children: make(map[patternTrieKey]*patternTrie),
	}
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
	return trie.Add(h)
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
	if !c.Nop && c.Call == nil {
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

		if !c.Nop {
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

	in, err := lexInput(input)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(input, CommentLeader) {
		cmd := &Command{Original: input, Nop: true}
		return cmd, nil
	}

	cmd := trie.compile(in.items)
	if cmd == nil {
		return nil, fmt.Errorf("invalid command: `%s`", input)
	}

	// patch original input
	cmd.Original = input

	return cmd, nil
}

// Compilef wraps fmt.Sprintf and Compile
func Compilef(format string, args ...interface{}) (*Command, error) {
	return Compile(fmt.Sprintf(format, args...))
}

// ExpandAliases finds the first alias match in input and replaces it with it's
// expansion.
func ExpandAliases(input string) string {
	aliasesLock.Lock()
	defer aliasesLock.Unlock()

	// find the first word in the input
	i := strings.IndexFunc(input, unicode.IsSpace)
	car, cdr := input, ""
	if i > 0 {
		car, cdr = input[:i], input[i:]
	}

	for k, v := range aliases {
		if k == car {
			log.Info("expanding %v -> %v", k, v)

			return v + cdr
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
	inputItems, err := lexInput(input)
	if err != nil {
		return fmt.Sprintf("unable to parse `%v`: %v", input, err)
	}

	if len(inputItems.items) == 0 {
		return printHelpShort(handlers)
	}

	matches := trie.help(inputItems.items)

	if len(matches) == 0 {
		return fmt.Sprintf("no help entry for `%s`", input)
	} else if len(matches) == 1 {
		return matches[0].helpLong()
	}

	// look for special case -- there are multiple handlers but only one has
	// long help text.
	count := 0
	for _, v := range matches {
		if len(v.HelpLong) > 0 {
			count += 1
		}
	}
	if count == 1 {
		handler := &Handler{}
		for _, v := range matches {
			handler.Patterns = append(handler.Patterns, v.Patterns...)
			if len(v.HelpLong) > 0 {
				handler.HelpLong = v.HelpLong
			}
		}
		handler.parsePatterns()
		return handler.helpLong()
	}

	return printHelpShort(matches)
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
