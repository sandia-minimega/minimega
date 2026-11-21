// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"strings"
	"testing"
)

func qemuImgReply() map[string]interface{} {
	return map[string]interface{}{
		"filename":     "/images/base.qcow2",
		"format":       "qcow2",
		"virtual-size": 10737418240.0,
		"actual-size":  1048576.0,
	}
}

func TestParseQemuImg(t *testing.T) {
	t.Run("without backing file", func(t *testing.T) {
		got, err := parseQemuImg(qemuImgReply())
		if err != nil {
			t.Fatal(err)
		}

		want := DiskInfo{
			Name:        "/images/base.qcow2",
			Format:      "qcow2",
			VirtualSize: 10737418240,
			DiskSize:    1048576,
		}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("with backing file", func(t *testing.T) {
		j := qemuImgReply()
		j["backing-filename"] = "base.qcow2"

		got, err := parseQemuImg(j)
		if err != nil {
			t.Fatal(err)
		}
		if got.BackingFile != "base.qcow2" {
			t.Errorf("backing file = %q, want %q", got.BackingFile, "base.qcow2")
		}
	})

	t.Run("unknown keys", func(t *testing.T) {
		j := qemuImgReply()
		j["dirty-flag"] = false
		j["cluster-size"] = 65536.0
		j["format-specific"] = map[string]interface{}{"type": "qcow2"}

		got, err := parseQemuImg(j)
		if err != nil {
			t.Fatal(err)
		}

		want, _ := parseQemuImg(qemuImgReply())
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	for _, key := range []string{"filename", "format", "virtual-size", "actual-size"} {
		t.Run("missing "+key, func(t *testing.T) {
			j := qemuImgReply()
			delete(j, key)

			if _, err := parseQemuImg(j); err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("expected missing %v error, got %v", key, err)
			}
		})
	}
}
