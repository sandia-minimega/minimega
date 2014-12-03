package minicli

import (
	"errors"
	"fmt"
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

type Command struct {
	Handler // Embeds the handler that was matched by the raw input

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
	items, err := lexPattern(h.Pattern)
	if err != nil {
		return err
	}

	h.patternItems = items
	handlers = append(handlers, h)

	return nil
}

// Process raw input text. An error is returned if parsing the input text
// failed.
func ProcessString(input string) (*Responses, error) {
	c, err := CompileCommand(input)
	if err != nil {
		return nil, err
	}
	return ProcessCommand(c)
}

// Process a prepopulated Command
func ProcessCommand(c *Command) (*Responses, error) {
	if c.Call == nil {
		return nil, fmt.Errorf("command %v has no callback!", c)
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
		prefix := handler.literalPrefix()
		if _, ok := groups[prefix]; !ok {
			groups[prefix] = make([]*Handler, 0)
		}

		groups[prefix] = append(groups[prefix], handler)
	}

	// User entered a valid command prefix as the argument to help, display help
	// for that group of handlers.
	if handlers, ok := groups[input]; input != "" && ok {
		if identicalHelp(handlers) {
			res := "Usage:\n"
			for _, handler := range handlers {
				res += "\t" + handler.Pattern + "\n"
			}
			res += "\n"
			res += handlers[0].HelpLong
			return res
		}

		// Weird case, share prefix but the help is not all the same. Print short
		// help for each except with full pattern in the left column.
		for _, handler := range handlers {
			helpShort[handler.Pattern] = handler.HelpShort
		}

		return printHelpShort(helpShort)
	}

	// Find the closest match for the input line, display the long help for it.
	handler, _ := closestMatch(inputItems)
	if handler != nil {
		res := "Usage: " + handler.Pattern
		res += "\n\n"
		res += handler.HelpLong
		return res
	}

	// List help for all the commands. Collapse handlers with the same string
	// literal prefix and help text into a single line.
	for prefix, handlers := range groups {
		if identicalHelp(handlers) {
			// Only append one help message for commands with the same prefix
			helpShort[prefix] = handlers[0].HelpShort
			continue
		}

		for _, handler := range handlers {
			helpShort[handler.Pattern] = handler.HelpShort
		}
	}

	return printHelpShort(helpShort)
}

func (c Command) String() string {
	return c.Original
}

// Return a string representation using the current output mode
// using the %v verb in pkg fmt
func (r Responses) String() {

}

// Return a verbose output representation for use with the %#v verb in pkg fmt
func (r Responses) GoString() {

}
