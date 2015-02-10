package minicli

import (
	"encoding/json"
	"errors"
	"fmt"
	log "minilog"
	"strings"
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

var (
	annotate bool // show hostnames in output
	compress bool // compress output
	headers  bool // show headers in output
	mode     int  // output mode
)

var handlers []*Handler
var history []string // command history for the write command

type Responses []*Response

// A response as populated by handler functions.
type Response struct {
	Host     string      // Host this response was created on
	Response string      // Simple response
	Header   []string    // Optional header. If set, will be used for both Response and Tabular data.
	Tabular  [][]string  // Optional tabular data. If set, Response will be ignored
	Error    string      // Because you can't gob/json encode an error type
	Data     interface{} // Optional user data
}

type CLIFunc func(*Command, chan Responses)

func init() {
	annotate = true
	compress = true
	headers = true
}

// Return any errors contained in the responses, or nil. If any responses have
// errors, the returned slice will be padded with nil errors to align the error
// with the response.
func (r Responses) Errors() []error {
	errs := make([]error, len(r))
	for i := range r {
		errs[i] = errors.New(r[i].Error)
	}

	return errs
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
	h.PatternItems = make([][]patternItem, len(h.Patterns))

	for i, pattern := range h.Patterns {
		items, err := lexPattern(pattern)
		if err != nil {
			return err
		}

		h.PatternItems[i] = items
	}

	h.HelpShort = strings.TrimSpace(h.HelpShort)
	h.HelpLong = strings.TrimSpace(h.HelpLong)

	handlers = append(handlers, h)

	return nil
}

// Process raw input text. An error is returned if parsing the input text
// failed.
func ProcessString(input string, record bool) (chan Responses, error) {
	c, err := CompileCommand(input)
	if err != nil {
		return nil, err
	}

	return ProcessCommand(c, record), nil
}

// Process a prepopulated Command
func ProcessCommand(c *Command, record bool) chan Responses {
	if c.Call == nil {
		log.Fatal("command %v has no callback!", c)
	}

	respChan := make(chan Responses)

	go func() {
		c.Call(c, respChan)

		// Append the command to the history
		if record {
			history = append(history, c.Original)
		}

		close(respChan)
	}()

	return respChan
}

// Create a command from raw input text. An error is returned if parsing the
// input text failed.
func CompileCommand(input string) (*Command, error) {
	inputItems, err := lexInput(input)
	if err != nil || len(inputItems) == 0 {
		return nil, err
	}

	_, cmd := closestMatch(inputItems)
	if cmd != nil {
		return cmd, nil
	}

	return nil, fmt.Errorf("invalid command: `%s`", input)
}

func Suggest(input string) []string {
	inputItems, err := lexInput(input)
	if err != nil {
		return nil
	}

	res := []string{}
	for _, h := range handlers {
		res = append(res, h.suggest(inputItems)...)
	}
	return res
}

//
func Help(input string) string {
	helpShort := make(map[string]string)

	inputItems, err := lexInput(input)
	if err != nil {
		return "Error parsing help input: " + err.Error()
	}

	// Figure out the literal string prefixes for each handler
	groups := make(map[string][]*Handler)
	for _, handler := range handlers {
		prefix := handler.Prefix()
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
		handler, _ := closestMatch(inputItems)
		if handler != nil {
			return handler.helpLong()
		}

		// Found an unresolvable command
		return fmt.Sprintf("no help entry for %s", input)
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
