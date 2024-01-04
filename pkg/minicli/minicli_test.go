// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minicli_test

import (
	"strings"
	"testing"

	. "github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// Valid patterns and inputs that should be acceptable, this is a list rather
// than a map because we need to ensure that some patterns are registered
// before others (specifically, those that support a subcommand).
var validTestPatterns = []struct {
	pattern string
	inputs  []string
}{
	// Optional list of strings
	{"ls [files]...", []string{"ls", "ls a", "ls a b", "ls a \"b c\" d"}},
	// Required list of strings plus required string
	{"mv <dest> <src>...", []string{"mv a b", "mv a b c", "mv a \"b c\" d"}},
	// String literal
	{"pwd", []string{"pwd"}},
	// String literal with spaces
	{"vm info", []string{"vm info"}},
	// Command that shares the same prefix with another command
	{"vm info search <terms>", []string{"vm info search foo"}},
	// Optional string
	{"cd [dir]", []string{"cd", "cd a"}},
	// Required string
	{"ping <host>", []string{"ping minimega.org"}},
	// Required multiple choice
	{"ip <addr,link>", []string{"ip addr", "ip link"}},
	// Required multiple choice followed by required string
	{"ip <addr,link> <command>...", []string{"ip addr add foo"}},
	// Optional multiple choice (we couldn't think of a real command
	{"foo [bar,zap]", []string{"foo", "foo bar", "foo zap"}},
	// Subcommand, must come last
	{"test (foo)", []string{"test cd", "test ping minimega.org", "test foo bar"}},
	// String literal, testing comments
	{"foobar", []string{"foobar # test", "foobar #test", "foobar#test", "foobar# test"}},
}

var invalidTestPatterns = []string{
	// Unterminated
	"ls (foo", "ls [foo", "ls <foo",
	// Weird nesting
	"ls (foo <bar>)", "ls [foo (bar)]", "ls <foo [bar]>",
	// Ambiguous optional fields
	"ls [foo] [bar]", "ls [foo,bar] [car]",
	// Messed up ellipsis
	"ls [foo].", "ls [foo]..", "ls [foo]....",
	// Weird trailing characters
	"ls [foo]bar", "ls <foo>bar", "ls (foo)bar",
	// Lists not at the end of pattern
	"ls [foo]... <bar>", "ls <foo>... <bar>",
	"ls [foo]... <bar>...", "ls <foo>... <bar>...",
	// Command not at the end of pattern
	"ls (foo) bar",
	// Spaces in multiple choices args
	"ls <foo, bar>", "ls <foo,bar baz,car>",
	// Quote in the pattern
	`ls "foo"`, `ls <foo bar "">`, `ls [foo 'bar']`, `ls (foo "roar")`,
}

var testPrefixes = []struct {
	Patterns []string
	Prefix   string
}{
	{
		Prefix: "vm info",
		Patterns: []string{
			"vm info",
			"vm info search",
			"vm info mask",
		},
	},
	{
		Prefix: "vm info",
		Patterns: []string{
			"vm info search",
			"vm info mask",
		},
	},
	{
		Prefix: "",
		Patterns: []string{
			"foo",
			"bar",
			"zombie",
		},
	},
}

var testHandler = &Handler{
	Patterns: []string{"test"},
	Call: func(c *Command, out chan<- Responses) {
		// Do nothing
	},
}

var echoArgHandler = &Handler{
	Patterns: []string{"<arg>"},
	Call: func(c *Command, out chan<- Responses) {
		resp := &Response{
			Response: c.StringArgs["arg"],
		}
		out <- Responses{resp}
	},
}

func TestParse(t *testing.T) {
	Reset()

	for _, v := range validTestPatterns {
		t.Logf("Testing pattern: `%s`", v.pattern)

		// Ensure that we can register the pattern without error
		err := Register(&Handler{Patterns: []string{v.pattern}})
		if err != nil {
			t.Errorf("unable to register `%v`: %v", v.pattern, err)
			continue
		}

		for _, i := range v.inputs {
			t.Logf("Testing input: `%s`", i)

			_, err := Compile(i)
			if err != nil {
				t.Errorf("unable to compile: %v", err)
			}
		}
	}
}

func TestInvalidPatterns(t *testing.T) {
	Reset()

	for _, p := range invalidTestPatterns {
		t.Logf("Testing pattern: `%s`", p)

		// Ensure that we can register the pattern without error
		err := Register(&Handler{Patterns: []string{p}})
		if err == nil {
			t.Errorf("accepting invalid pattern: `%s`", p)
		}
	}
}

func TestPrefix(t *testing.T) {
	Reset()

	for i := range testPrefixes {
		want := testPrefixes[i].Prefix
		patterns := testPrefixes[i].Patterns

		t.Logf("Testing patterns: %q", patterns)

		for _ = range patterns {
			// Shuffle the patterns left one place
			first := patterns[0]
			for j := 0; j < len(patterns)-1; j++ {
				patterns[j] = patterns[(j+1)%len(patterns)]
			}
			patterns[len(patterns)-1] = first

			handler := &Handler{Patterns: patterns}
			Register(handler) // populate patternItems

			got := handler.SharedPrefix
			if got != want {
				t.Errorf("got `%s`, wanted `%s`", got, want)
				break
			}
		}
	}
}

func TestHistoryComments(t *testing.T) {
	Reset()
	Register(testHandler)

	comments := []string{
		"test #one",
		"test # two",
		"test#three",
		"# four",
	}

	for _, c := range comments {
		out, err := ProcessString(c, true)
		if err != nil {
			t.Fatalf("unable to ProcessString: %s -- %v", c, err)
		}

		for _ = range out {
			// drop responses
		}
	}

	got := History()
	want := strings.Join(comments, "\n")

	if got != want {
		t.Error("got incorrect history")
		t.Logf("got:\n`%s`", got)
		t.Logf("want:\n`%s`", want)
	}
}

func TestWhitespace(t *testing.T) {
	Reset()
	Register(testHandler)

	inputs := []string{
		"test",
		" test",
		"\ttest",
		"\ttest\t",
		"  ",
		"# test",
		" # test",
		"\t# test",
		"\t# test\t",
	}

	for _, v := range inputs {
		t.Logf("processing input: `%s`", v)

		out, err := ProcessString(v, true)
		if err != nil {
			t.Fatalf("unable to ProcessString: `%s` -- %v", v, err)
		}

		for _ = range out {
			// drop responses
		}
	}

	t.Logf("history:\n`%s`", History())
}

func TestQuotes(t *testing.T) {
	Reset()
	Register(echoArgHandler)

	inputs := [][]string{
		[]string{`foo`, `foo`},
		[]string{`"foo"`, `foo`},
		[]string{`"foo bar"`, `foo bar`},
		[]string{`"\"foo bar\""`, `"foo bar"`},
		[]string{`"\"foo's bar\""`, `"foo's bar"`},
		[]string{`'foo'`, `foo`},
		[]string{`'foo bar'`, `foo bar`},
		[]string{`'"foo bar"'`, `"foo bar"`},
		[]string{`'"foo\'s bar"'`, `"foo's bar"`},
		[]string{`"foo \"bar\""`, `foo "bar"`},
		[]string{`""`, ``},
	}

	for _, v := range inputs {
		t.Logf("processing input: `%s`, want: `%v`", v[0], v[1])

		out, err := ProcessString(v[0], true)
		if err != nil {
			t.Fatalf("unable to ProcessString: `%s` -- %v", v[0], err)
		}

		for r := range out {
			if len(r) != 1 {
				t.Errorf("expected one response")
				continue
			}

			if r[0].Response != v[1] {
				t.Errorf("quote mismatch: `%v` != `%v`", r[0].Response, v[1])
			}
		}
	}
}
