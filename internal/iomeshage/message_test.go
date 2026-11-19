// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package iomeshage

import (
	"testing"
	"time"
)

// In -headnode mode the head node's copy is used unless the local file has
// the same hash. A file with no hash yet (still changing, or empty) must not
// make a missing local file look up to date.
func TestFilesUseHeadNode(t *testing.T) {
	tests := []struct {
		name      string
		headHash  string
		localHash string
		local     bool
		wantLocal bool
	}{
		{"same hash", "abc", "abc", true, true},
		{"different hash", "abc", "def", true, false},
		{"missing locally", "abc", "", false, false},
		{"local not hashed yet", "abc", "", true, false},
		{"head not hashed, missing locally", "", "", false, false},
		{"neither hashed", "", "", true, true},
	}

	for _, tt := range tests {
		files := NewFiles("head", true)
		files.add(&Message{From: "other", Filename: "foo", Hash: "zzz"})
		files.add(&Message{From: "head", Filename: "foo", Hash: tt.headHash})

		msg, ok := files.use("foo", tt.localHash, tt.local)
		if !ok {
			t.Errorf("%v: no decision", tt.name)
			continue
		}

		if gotLocal := msg == nil; gotLocal != tt.wantLocal {
			t.Errorf("%v: use local = %v, want %v", tt.name, gotLocal, tt.wantLocal)
		} else if msg != nil && msg.From != "head" {
			t.Errorf("%v: file taken from %v, want the head node", tt.name, msg.From)
		}
	}

	// a file the head node does not have: keep the local copy if there is one
	files := NewFiles("head", true)
	files.add(&Message{From: "other", Filename: "foo", Hash: "zzz"})

	if msg, ok := files.use("foo", "", true); !ok || msg != nil {
		t.Errorf("file not on head node, present locally: got %v, %v", msg, ok)
	}
	if msg, ok := files.use("foo", "", false); !ok || msg == nil || msg.From != "other" {
		t.Errorf("file not on head node, missing locally: got %v, %v", msg, ok)
	}
}

// Without a head node, differing hashes select the newest copy.
func TestFilesUseNewest(t *testing.T) {
	now := time.Now()

	files := NewFiles("", true)
	files.add(&Message{From: "old", Filename: "foo", Hash: "abc", ModTime: now.Add(-time.Hour)})
	files.add(&Message{From: "new", Filename: "foo", Hash: "def", ModTime: now})
	files.add(&Message{From: "older", Filename: "foo", Hash: "abc", ModTime: now.Add(-2 * time.Hour)})

	if msg, ok := files.use("foo", "", false); !ok || msg == nil || msg.From != "new" {
		t.Errorf("got %v, %v, want the newest copy", msg, ok)
	}

	// -headnode without -hashfiles behaves as if no head node was set
	files = NewFiles("head", false)
	files.add(&Message{From: "other", Filename: "foo"})
	files.add(&Message{From: "head", Filename: "foo"})

	if msg, ok := files.use("foo", "", false); !ok || msg == nil || msg.From != "other" {
		t.Errorf("got %v, %v, want the first copy reported", msg, ok)
	}

	if _, ok := files.use("glob*", "", false); ok {
		t.Error("a path nobody reported got a decision")
	}
}
