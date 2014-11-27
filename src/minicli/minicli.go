// minicli is a command line interface backend for minimega. It allows
// registering handlers and function callbacks for command line arguments that
// match defined patterns.
//
// minicli also supports multiple output rendering modes and stream and tabular
// compression.
package minicli

import (
	"bufio"
	"errors"
	"strings"
)

// Output modes
const (
	MODE_NORMAL = iota
	MODE_JSON
	MODE_QUIET
)

var (
	compress bool // compress output
	tabular  bool // tabularize output
	mode     int  // output mode
)

var registeredPatterns [][]patternItem

type Command struct {
	Original   string              // original raw input
	Pattern    string              // the pattern we matched
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
	registeredPatterns = make([][]patternItem, 0)
}

// Register a new API based on pattern. Patterns consist of required text, required and optional fields, multiple choice arguments, and variable number of arguments. The pattern syntax is as follows:
// <foo> 	a required string, returned in the arg map with key "foo"
// <foo bar> 	a required string, still returned in the arg map with key "foo".
//	 	The extra is just documentation
// foo bar	literal required text, as in "capture netflow <foo>"
// [foo]	optional string, returned in the arg map with key "foo". There can
// 		be only one optional arg and it must be at the end of the pattern.
// <foo,bar>	a required multiple choice argument. Returned as whichever
// 		choice is made in the argmap (the argmap key is simply created).
// [foo,bar]	an optional multiple choice argument.
// <foo>...	a required list of strings, one or more, with the key "foo" in
// 		the argmap
// [foo]...	an optional list of strings, zero or more, with the key "foo" in
// 		the argmap. This is the only way to support multiple optional fields.
// (foo) a subcommand that must also be valid. Must be at the end of pattern.
func Register(pattern string, handler func(*Command) *Responses) error {
	s := bufio.NewScanner(strings.NewReader(pattern))
	s.Split(bufio.ScanRunes)
	l := patternLexer{s: s, items: make([]patternItem, 0)}

	err := l.Run()
	if err != nil {
		return err
	}

	registeredPatterns = append(registeredPatterns, l.items)

	return nil
}

// Process raw input text. An error is returned if parsing the input text
// failed.
func ProcessString(input string) (*Responses, error) {
	c, err := CompileCommand(input)
	if err != nil {
		return nil, err
	}
	return ProcessCommand(c), nil
}

// Process a prepopulated Command
func ProcessCommand(c *Command) *Responses {
	return nil
}

// Create a command from raw input text. An error is returned if parsing the
// input text failed.
func CompileCommand(input string) (*Command, error) {
	s := bufio.NewScanner(strings.NewReader(input))
	s.Split(bufio.ScanRunes)
	l := inputLexer{s: s, items: make([]inputItem, 0)}

	err := l.Run()
	if err != nil {
		return nil, err
	}

	cmd := Command{Original: input,
		StringArgs: make(map[string]string),
		BoolArgs:   make(map[string]bool),
		ListArgs:   make(map[string][]string)}

	// Keep track of what was the closest
	var closestPattern []patternItem
	var longestMatch int

outer:
	for _, pattern := range registeredPatterns {
		for i, pItem := range pattern {
			// We ran out of items before matching all the items in the pattern
			if len(l.items) <= i {
				// Check if the remaining item is optional
				if pItem.Type == optString || pItem.Type == optList || pItem.Type == optChoice {
					// Matched!
					break
				}

				continue outer
			}

			switch pItem.Type {
			case literalString:
				if l.items[i].Value != pItem.Text {
					continue outer
				}
			case reqString, optString:
				cmd.StringArgs[pItem.Key] = l.items[i].Value
			case reqChoice, optChoice:
				var found bool
				for _, choice := range pItem.Options {
					if choice == l.items[i].Value {
						cmd.BoolArgs[choice] = true
						found = true
					}
				}

				if !found {
					// Invalid choice
					continue outer
				}
			case reqList, optList:
				res := make([]string, len(l.items))
				for i, v := range l.items {
					res[i] = v.Value
				}

				cmd.ListArgs[pItem.Key] = res
			case cmdString:
				// Parse the subcommand
				subCmd, err := CompileCommand(printInput(l.items[i:]))
				if err != nil {
					return nil, err
				}

				cmd.Subcommand = subCmd
			}

			if i > longestMatch {
				closestPattern = pattern
				longestMatch = i
			}
		}

		cmd.Pattern = printPattern(pattern)
		return &cmd, nil
	}

	// TODO: Do something with closestPattern
	_ = closestPattern

	return nil, errors.New("no matching commands found")
}

// List installed patterns and handlers
func Handlers() string {
	return ""
}

// Enable or disable response compression
func CompressOutput(compress bool) {

}

// Enable or disable tabular aggregation
func TabularOutput(tabular bool) {

}

// Return a string representation using the current output mode
// using the %v verb in pkg fmt
func (r Responses) String() {

}

// Return a verbose output representation for use with the %#v verb in pkg fmt
func (r Responses) GoString() {

}

// Return any errors contained in the responses, or nil. If any responses have
// errors, the returned slice will be padded with nil errors to align the error
// with the response.
func (r Responses) Errors() []error {
	return nil
}

// Set the output mode for String()
func OutputMode(mode int) {

}
