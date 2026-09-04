// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestMarkdownEscape(t *testing.T) {
	input := "Stored in <filepath>/<vm id>.\n#: playback comment\nIndented # text\n\n    capture pcap snaplen <size>\n\tcapture pcap filter <bpf>"
	want := `Stored in &lt;filepath&gt;/&lt;vm id&gt;.
\#: playback comment
Indented # text

    capture pcap snaplen <size>
	capture pcap filter <bpf>`

	if got := markdownEscape(input); got != want {
		t.Fatalf("markdownEscape() = %q, want %q", got, want)
	}
}

func TestAPITemplatesIncludeDate(t *testing.T) {
	api := apigen{
		Date:     "2 September 2026",
		Sections: map[string][]*minicli.Handler{},
	}

	for _, name := range []string{"minimega_api.template", "minirouter_api.template"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "doc", "content_templates", name)
			tmpl, err := template.New(name).Funcs(template.FuncMap{
				"markdownEscape": markdownEscape,
			}).ParseFiles(path)
			if err != nil {
				t.Fatal(err)
			}

			var out bytes.Buffer
			if err := tmpl.Execute(&out, api); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Last updated 2 September 2026.") {
				t.Fatalf("generated API page does not include last updated date")
			}
		})
	}
}
