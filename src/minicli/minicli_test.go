package minicli_test

import (
	. "minicli"
	"testing"
)

var validTestPatterns = map[string][]string{
	// Optional list of strings
	"ls [files]...": []string{"ls", "ls a", "ls a b", "ls a \"b c\" d"},
	// Required list of strings plus required string
	"mv <src>... <dest>": []string{"mv a b", "mv a b c", "mv a \"b c\" d"},
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
}

func TestParse(t *testing.T) {
	for k, v := range validTestPatterns {
		err := Register(k, nil)
		if err != nil {
			t.Errorf(err.Error())
		}
		for _, s := range v {
			_, err := ProcessString(s)
			if err != nil {
				t.Errorf(err.Error())
			}
		}
	}
}
