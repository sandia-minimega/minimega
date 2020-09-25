package image

import (
	"bytes"
	"phenix/tmpl"
	v1 "phenix/types/version/v1"
	"strings"
	"testing"
)

func TestImageTemplate(t *testing.T) {
	img := v1.Image{
		Variant:     "brash",
		Release:     "bionic",
		Format:      "qcow2",
		Size:        "10G",
		Mirror:      "http://us.archive.ubuntu.com",
		Packages:    []string{"wireshark"},
		Overlays:    []string{"bennu_overlay"},
		VerboseLogs: true,
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
