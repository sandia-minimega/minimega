package minicli

import "errors"

type OutputMode int

// Output modes
const (
	DefaultMode OutputMode = iota
	JsonMode
	QuietMode
)

var (
	compress bool       // compress output
	tabular  bool       // tabularize output
	mode     OutputMode // output mode
)

var handlers []Handler

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
	handlers = make([]Handler, 0)
}

// Register a new API based on pattern. See package documentation for details
// about supported patterns.
func Register(h Handler) error {
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
	return ProcessCommand(c), nil
}

// Process a prepopulated Command
func ProcessCommand(c *Command) *Responses {
	return nil
}

// Create a command from raw input text. An error is returned if parsing the
// input text failed.
func CompileCommand(input string) (*Command, error) {
	inputItems, err := lexInput(input)
	if err != nil {
		return nil, err
	}

	// Keep track of what was the closest
	var closestHandler Handler
	var longestMatch int

	for _, h := range handlers {
		cmd, matchLen := h.compileCommand(inputItems)
		if cmd != nil {
			cmd.Original = input
			return cmd, nil
		}

		if matchLen > longestMatch {
			closestHandler = h
			longestMatch = matchLen
		}
	}

	// TODO: Do something with closestHandler
	_ = closestHandler

	return nil, errors.New("no matching commands found")
}

// List installed patterns and handlers
func Handlers() string {
	return ""
}

// Enable or disable response compression
func CompressOutput(flag bool) {
	compress = flag
}

// Enable or disable tabular aggregation
func TabularOutput(flag bool) {
	tabular = flag
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
func SetOutputMode(newMode OutputMode) {
	mode = newMode
}
