// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

var invalidNamespaceNames = []string{"", ".", "..", "foo/bar", "../foo", "ns/../../etc"}

func TestIsValidName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// acceptable names
		{"foo", true},
		{"ns1", true},
		{"foo-bar_baz.1", true},
		{".hidden", true},
		{"a.", true},
		{"...", true},
		// empty
		{"", false},
		// filesystem-special names that would escape or alias a directory
		{".", false},
		{"..", false},
		// disallowed characters
		{"foo bar", false},
		{"foo/bar", false},
		{"../foo", false},
		{"ns/../../etc", false},
		{"foo\x00", false},
	}

	for _, tt := range tests {
		if got := isValidName(tt.name); got != tt.want {
			t.Errorf("isValidName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// namespaceExists reports whether name is in the namespace registry.
func namespaceExists(name string) bool {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	_, ok := namespaces[name]
	return ok
}

// TestGetOrCreateNamespaceRejectsInvalidNames makes sure that no path that
// creates a namespace can create one with a name that is not valid.
func TestGetOrCreateNamespaceRejectsInvalidNames(t *testing.T) {
	for _, name := range invalidNamespaceNames {
		ns, err := GetOrCreateNamespace(name)
		if !errors.Is(err, validNameErr) {
			t.Errorf("GetOrCreateNamespace(%q) error = %v, want %v", name, err, validNameErr)
		}

		if ns != nil {
			t.Errorf("GetOrCreateNamespace(%q) = %v, want nil", name, ns)
		}

		if namespaceExists(name) {
			t.Errorf("GetOrCreateNamespace(%q) registered the namespace", name)
		}
	}
}

func TestGetOrCreateNamespaceReturnsExisting(t *testing.T) {
	registerNamespace(t, "existing")

	ns, err := GetOrCreateNamespace("existing")
	if err != nil {
		t.Fatalf("GetOrCreateNamespace: %v", err)
	}

	if ns.Name != "existing" {
		t.Errorf("GetOrCreateNamespace returned %q, want %q", ns.Name, "existing")
	}
}

// TestNamespaceQueueRejectsInvalidVMNames makes sure that a VM name that is
// not valid is not queued, and that a valid name is.
func TestNamespaceQueueRejectsInvalidVMNames(t *testing.T) {
	ns := &Namespace{Name: "queue-names"}

	for _, name := range []string{".", "..", "foo/bar", "foo,.."} {
		err := ns.Queue(name, KVM, VMConfig{})
		if err == nil || !strings.Contains(err.Error(), validNameErr.Error()) {
			t.Errorf("Queue(%q) error = %v, want an error with %q", name, err, validNameErr)
		}

		if len(ns.queue) != 0 {
			t.Fatalf("Queue(%q) queued %v", name, ns.queue[0].Names)
		}
	}

	if err := ns.Queue("foo", KVM, VMConfig{}); err != nil {
		t.Fatalf("Queue(%q): %v", "foo", err)
	}

	if len(ns.queue) != 1 || len(ns.queue[0].Names) != 1 || ns.queue[0].Names[0] != "foo" {
		t.Errorf("queue = %+v, want one entry for foo", ns.queue)
	}
}

// TestWrapSimpleCLIRejectsInvalidNamespace makes sure that a command tagged
// with an invalid namespace, for example one received over meshage, gets an
// error response and does not run.
func TestWrapSimpleCLIRejectsInvalidNamespace(t *testing.T) {
	var called bool

	handler := wrapSimpleCLI(func(*Namespace, *minicli.Command, *minicli.Response) error {
		called = true
		return nil
	})

	respChan := make(chan minicli.Responses, 1)
	handler(&minicli.Command{Original: "vm info", Namespace: ".."}, respChan)

	resps := <-respChan
	if len(resps) != 1 || !strings.Contains(resps[0].Error, validNameErr.Error()) {
		t.Errorf("responses = %+v, want one response with error %q", resps, validNameErr)
	}

	if called {
		t.Error("handler ran for a command with an invalid namespace")
	}

	if namespaceExists("..") {
		t.Error(`namespace ".." was created`)
	}
}

// TestMeshageVMLaunchResponseRejectsInvalidNamespace makes sure that a VM
// launch received over meshage with an invalid namespace does not create the
// namespace and sends the error back to the scheduler.
func TestMeshageVMLaunchResponseRejectsInvalidNamespace(t *testing.T) {
	for _, name := range invalidNamespaceNames {
		resp := meshageVMLaunchResponse(meshageVMLaunch{
			QueuedVMs: &QueuedVMs{},
			Namespace: name,
			TID:       42,
			From:      "scheduler",
		})

		if resp.TID != 42 {
			t.Errorf("namespace %q: TID = %v, want 42", name, resp.TID)
		}

		if len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0], validNameErr.Error()) {
			t.Errorf("namespace %q: errors = %q, want one error with %q", name, resp.Errors, validNameErr)
		}

		if namespaceExists(name) {
			t.Errorf("namespace %q was created", name)
		}
	}
}
