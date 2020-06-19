package image

import (
	"bytes"
	"fmt"
	"phenix/tmpl"
	v1 "phenix/types/version/v1"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestImageTemplate(t *testing.T) {
	img := v1.Image{
		Variant:   "brash",
		Release:   "bionic",
		Format:    "qcow2",
		Size:      "10G",
		Mirror:    "http://us.archive.ubuntu.com",
		Packages:  []string{"wireshark"},
		Overlays:  []string{"bennu_overlay"},
		Verbosity: "--verbose",
	}

	if err := SetDefaults(&img); err != nil {
		t.Log(err)
		t.FailNow()
	}

	var buf bytes.Buffer

	if err := tmpl.GenerateFromTemplate("vmdb.tmpl", img, &buf); err != nil {
		t.Log(err)
		t.FailNow()
	}

	t.Log(buf.String())

	if !strings.Contains(buf.String(), `options: "--include wireshark`) {
		t.Log("missing packages in options")
		t.FailNow()
	}
}

func TestScriptRemoval(t *testing.T) {
	img := v1.Image{
		Variant:   "brash",
		Release:   "bionic",
		Format:    "qcow2",
		Size:      "10G",
		Mirror:    "http://us.archive.ubuntu.com",
		Packages:  []string{"wireshark"},
		Overlays:  []string{"bennu_overlay"},
		Verbosity: "--verbose",
	}

	for _, l := range strings.Split(POSTBUILD_APT_CLEANUP, "\n") {
		img.Scripts = append(img.Scripts, l)
	}

	custom := []string{
		"## foo/bar.sh START ##",
		"#!/bin/bash",
		`echo "foo bar!"`,
		"## foo/bar.sh END ##",
	}

	img.Scripts = append(img.Scripts, custom...)

	body, _ := yaml.Marshal(img)

	t.Log(string(body))

	scripts := []string{"foo/bar.sh"}

	for _, p := range scripts {
		var (
			matcher = fmt.Sprintf(SCRIPT_START_COMMENT, p)
			start   = -1
			end     = -1
		)

		for i, l := range img.Scripts {
			if l == matcher {
				if start < 0 {
					start = i
					matcher = fmt.Sprintf(SCRIPT_END_COMMENT, p)
				} else {
					end = i
					break
				}
			}
		}

		if start >= 0 && end > 0 {
			img.Scripts = append(img.Scripts[:start], img.Scripts[end+1:]...)
		}
	}

	body, _ = yaml.Marshal(img)

	t.Log(string(body))
}
