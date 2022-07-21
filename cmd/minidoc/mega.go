// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sandia-minimega/minimega/v2/internal/present"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type Mega struct {
	Text     template.HTML
	Filename string
	Height   int
}

func init() {
	present.Register("mega", parseMega)
}

func (c Mega) TemplateName() string { return "mega" }

func executable(m Mega) bool {
	return *f_exec
}

func parseMega(ctx *present.Context, sourceFile string, sourceLine int, cmd string) (present.Elem, error) {
	cmd = strings.TrimSpace(cmd)
	log.Debug("parseMega cmd: %v", cmd)

	f := strings.Fields(cmd)

	if len(f) != 2 && len(f) != 3 {
		return nil, fmt.Errorf("invalid .mega directive: %v", cmd)
	}

	var height int
	if len(f) == 3 {
		h, err := strconv.Atoi(f[2])
		if err != nil {
			return nil, err
		}
		height = h
		log.Debug("got mega height: %v", h)
	}

	filename := filepath.Join(*f_root, filepath.Dir(sourceFile), f[1])
	log.Debug("filename: %v", filename)

	text, err := ctx.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("%s:%d: %v", sourceFile, sourceLine, err)
	}

	data := &megaTemplateData{
		Body: string(text),
	}

	var buf bytes.Buffer
	if err := megaTemplate.Execute(&buf, data); err != nil {
		return nil, err
	}

	return Mega{
		Text:     template.HTML(buf.String()),
		Filename: filepath.Base(filename),
		Height:   height,
	}, nil
}

type megaTemplateData struct {
	Body string
}

var megaTemplate = template.Must(template.New("code").Parse(megaTemplateHTML))

const megaTemplateHTML = `
<pre contenteditable="true" spellcheck="false">
{{.Body}}
</pre>
`
