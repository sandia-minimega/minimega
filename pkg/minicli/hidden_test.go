// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minicli_test

import (
	"strings"
	"testing"

	. "github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestHiddenHandler(t *testing.T) {
	Reset()
	defer Reset()

	var ran bool

	MustRegister(&Handler{
		HelpShort: "a visible command",
		Patterns:  []string{"visible"},
		Call: func(c *Command, out chan<- Responses) {
			out <- Responses{&Response{Response: "visible"}}
		},
	})
	MustRegister(&Handler{
		HelpShort: "a secret command",
		Patterns:  []string{"secretcmd"},
		Hidden:    true,
		Call: func(c *Command, out chan<- Responses) {
			ran = true
			out <- Responses{&Response{Response: "secret"}}
		},
	})

	out, err := ProcessString("secretcmd", false)
	if err != nil {
		t.Fatalf("hidden handler did not parse: %v", err)
	}

	var got []string
	for resps := range out {
		for _, r := range resps {
			got = append(got, r.Response)
		}
	}
	if !ran || len(got) != 1 || got[0] != "secret" {
		t.Errorf("hidden handler did not run: ran=%v responses=%v", ran, got)
	}

	if help := Help(""); strings.Contains(help, "secretcmd") || !strings.Contains(help, "visible") {
		t.Errorf("help listing shows hidden handler or omits visible one:\n%v", help)
	}

	if help := Help("secretcmd"); strings.Contains(help, "a secret command") {
		t.Errorf("command help shows hidden handler:\n%v", help)
	}

	for _, s := range Suggest("sec") {
		if s == "secretcmd" {
			t.Errorf("suggestions include hidden handler: %v", s)
		}
	}

	doc, err := Doc()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc, "secretcmd") || !strings.Contains(doc, "visible") {
		t.Errorf("doc output shows hidden handler or omits visible one: %v", doc)
	}
}
