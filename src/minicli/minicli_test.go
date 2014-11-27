package minicli_test

import (
	. "minicli"
	"testing"
)

var validTestPatterns = map[string][]string{
	// Optional list of strings
	"ls [files]...": []string{"ls", "ls a", "ls a b", "ls a \"b c\" d"},
	// Required list of strings plus required string
	"mv <dest> <src>...": []string{"mv a b", "mv a b c", "mv a \"b c\" d"},
	// String literal
	"pwd": []string{"pwd"},
	// String literal with spaces
	"vm info": []string{"vm info"},
	// Optional string
	"cd [dir]": []string{"cd", "cd a"},
	// Required string
	"ping <host>": []string{"ping minimega.org"},
	// Required string w/ comment
	"ping6 <host hostname>": []string{"ping6 minimega.org"},
	// Required multiple choice
	"ip <addr,link>": []string{"ip addr", "ip link"},
	// Optional multiple choice (we couldn't think of a real command
	"foo [bar,zap]": []string{"foo", "foo bar", "foo zap"},
	// Subcommand
	"test (foo)": []string{"test cd", "test ping minimega.org", "test foo bar"},
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

func TestParse(t *testing.T) {
	for k, v := range validTestPatterns {
		t.Logf("Testing pattern: `%s`", k)

		// Ensure that we can register the pattern without error
		err := Register(k, nil)
		if err != nil {
			t.Errorf(err.Error())
			continue
		}

		for _, s := range v {
			t.Logf("Testing input: `%s`", s)

			cmd, err := CompileCommand(s)
			if err != nil {
				t.Errorf("unable to compile command, %s", err.Error())
			} else if cmd.Pattern != k {
				t.Errorf("unexpected match, `%s` != `%s`", k, cmd.Pattern)
			}
		}
	}
}

func TestInvalidPatterns(t *testing.T) {
	for _, p := range invalidTestPatterns {
		t.Logf("Testing pattern: `%s`", p)

		// Ensure that we can register the pattern without error
		err := Register(p, nil)
		if err == nil {
			t.Errorf("accepting invalid pattern: `%s`", p)
		}
	}
}
