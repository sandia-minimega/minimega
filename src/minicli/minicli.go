package minicli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
)

// Output modes
const (
	DefaultMode = iota
	JsonMode
	QuietMode
)

var (
	compress bool // compress output
	tabular  bool // tabularize output
	mode     int  // output mode
)

var handlers []*Handler
var history []string // command history for the write command

type Command struct {
	Handler // Embeds the handler that was matched by the raw input

	Pattern    string              // the specific pattern that was matched
	Original   string              // original raw input
	StringArgs map[string]string   // map of arguments
	BoolArgs   map[string]bool     // map of arguments
	ListArgs   map[string][]string // map of arguments
	Subcommand *Command            // parsed command
}

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

func init() {
	handlers = make([]*Handler, 0)
	history = make([]string, 0)
}

// Enable or disable response compression
func CompressOutput(flag bool) {
	compress = flag
}

// Enable or disable tabular aggregation
func TabularOutput(flag bool) {
	tabular = flag
}

// Set the output mode for String()
func SetOutputMode(newMode int) {
	mode = newMode
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

// Register a new API based on pattern. See package documentation for details
// about supported patterns.
func Register(h *Handler) error {
	h.patternItems = make([][]patternItem, len(h.Patterns))

	for i, pattern := range h.Patterns {
		items, err := lexPattern(pattern)
		if err != nil {
			return err
		}

		h.patternItems[i] = items
	}

	handlers = append(handlers, h)

	return nil
}

// Process raw input text. An error is returned if parsing the input text
// failed.
func ProcessString(input string) (Responses, error) {
	c, err := CompileCommand(input)
	if err != nil {
		return nil, err
	}
	return ProcessCommand(c)
}

// Process a prepopulated Command
func ProcessCommand(c *Command) (Responses, error) {
	if c.Call == nil {
		return nil, fmt.Errorf("command %v has no callback!", c)
	}

	// Append the command to the history
	if c.Record {
		history = append(history, c.Original)
	}

	return c.Call(c), nil
}

// Create a command from raw input text. An error is returned if parsing the
// input text failed.
func CompileCommand(input string) (*Command, error) {
	inputItems, err := lexInput(input)
	if err != nil {
		return nil, err
	}

	_, cmd := closestMatch(inputItems)
	if cmd != nil {
		return cmd, nil
	}

	return nil, errors.New("no matching commands found")
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
	if handlers, ok := groups[input]; input != "" && ok {
		// Only one handler with a given pattern prefix, give the long help message
		if len(handlers) == 1 {
			return handlers[0].helpLong()
		}

		// Weird case, multiple handlers share the same prefix. Print the short
		// help for each handler for each pattern registered.
		// TODO: Is there something better we can do?
		for _, handler := range handlers {
			for _, pattern := range handler.Patterns {
				helpShort[pattern] = handler.helpShort()
			}
		}

		return printHelpShort(helpShort)
	}

	// If there's a closest match, display the long help for it
	// TODO: Maybe we want to allow for partial matches here... there's a weird
	// bug when you type in the suffix of several handlers (e.g. "help vm")
	handler, _ := closestMatch(inputItems)
	if handler != nil {
		return handler.helpLong()
	}

	// List help for all the commands. Collapse handlers with the same prefix and
	// into a single line. If there's multiple handlers that share the same
	// prefix, use the full pattern instead of the prefix.
	for prefix, handlers := range groups {
		if len(handlers) == 1 {
			helpShort[prefix] = handlers[0].helpShort()
		} else {
			for _, handler := range handlers {
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

// Return a string representation using the current output mode
// using the %v verb in pkg fmt
func (r Responses) String() string {
	if len(r) == 0 {
		return ""
	}

	header, err := r.getHeader()
	if err != nil {
		return err.Error()
	}

	// TODO: What is Header for simple responses?

	tabular, err := r.validTabular(header)
	if err != nil {
		return err.Error()
	}

	var buf bytes.Buffer

	if tabular {
		w := new(tabwriter.Writer)
		w.Init(&buf, 5, 0, 1, ' ', 0)
		for i, h := range header {
			if i != 0 {
				fmt.Fprintf(w, "\t| ")
			}
			fmt.Fprintf(w, h)
		}

		// Print out the tabular data for all responses that don't have an error
		for i := range r {
			for j := range r[i].Tabular {
				for k, val := range r[i].Tabular[j] {
					if k != 0 {
						fmt.Fprintf(w, "\t| ")
					}
					fmt.Fprintf(w, val)
				}
				fmt.Fprintf(w, "\n")
			}
		}
		w.Flush()
	} else {
		for i := range r {
			if r[i].Error == "" {
				buf.WriteString(r[i].Response)
				buf.WriteString("\n")
			}
		}
	}

	// Append errors from hosts
	var hasErrors bool
	for i := range r {
		if r[i].Error != "" {
			// Found first error
			if !hasErrors {
				buf.WriteString("\n")
				buf.WriteString("Errors:")
				buf.WriteString("\n")
				hasErrors = true
			}

			// TODO: Use tabwriter?
			buf.WriteString("\t")
			buf.WriteString(r[i].Host)
			buf.WriteString("\t")
			buf.WriteString(r[i].Error)
			buf.WriteString("\n")
		}
	}

	resp := buf.String()
	return strings.TrimSpace(resp)
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
