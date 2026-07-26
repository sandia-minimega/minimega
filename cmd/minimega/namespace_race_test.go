// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"sync"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// registerNamespace inserts a bare namespace into the registry. NewNamespace
// reaches into meshageNode, which only exists in a running daemon, and none of
// these tests need a functional namespace -- only a registered name.
func registerNamespace(t *testing.T, name string) {
	t.Helper()

	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	if _, ok := namespaces[name]; !ok {
		namespaces[name] = &Namespace{Name: name}
	}
}

// TestResolveNamespaceFallsBackToActive covers the compatibility half of the
// contract: a command carrying no namespace of its own must resolve exactly as
// it always did, via the process-wide active namespace. The interactive CLI,
// raw `mesh send` and scripts that rely on ambient state all depend on this.
func TestResolveNamespaceFallsBackToActive(t *testing.T) {
	registerNamespace(t, DefaultNamespace)

	defer func() { _ = SetNamespace(DefaultNamespace) }()

	registerNamespace(t, "ambient")

	if err := SetNamespace("ambient"); err != nil {
		t.Fatalf("SetNamespace: %v", err)
	}

	if got := resolveNamespace(nil).Name; got != "ambient" {
		t.Errorf("resolveNamespace(nil) = %q, want %q", got, "ambient")
	}

	untagged := &minicli.Command{Original: "vm info"}

	if got := resolveNamespace(untagged).Name; got != "ambient" {
		t.Errorf("untagged command resolved to %q, want %q", got, "ambient")
	}
}

// TestResolveNamespacePrefersTheTag covers the other half: once a command has
// been told which namespace it belongs to, the active namespace is irrelevant.
func TestResolveNamespacePrefersTheTag(t *testing.T) {
	registerNamespace(t, DefaultNamespace)

	defer func() { _ = SetNamespace(DefaultNamespace) }()

	registerNamespace(t, "ambient")
	registerNamespace(t, "tagged")

	if err := SetNamespace("ambient"); err != nil {
		t.Fatalf("SetNamespace: %v", err)
	}

	cmd := &minicli.Command{Original: "vm info", Namespace: "tagged"}

	if got := resolveNamespace(cmd).Name; got != "tagged" {
		t.Errorf("tagged command resolved to %q, want %q", got, "tagged")
	}
}

// TestSetNamespaceTagsNestedSubcommands guards the fan-out path. A command is
// only safe if every level of it carries the tag: wrapBroadcastCLI recompiles
// the local leg from scratch and dispatches it separately, so a tag that stops
// at the top level leaves the leg that does the actual work resolving through
// the racy global.
func TestSetNamespaceTagsNestedSubcommands(t *testing.T) {
	inner := &minicli.Command{Original: "vm info"}
	middle := &minicli.Command{Original: "namespace bar vm info", Subcommand: inner}
	outer := &minicli.Command{Original: "namespace foo ...", Subcommand: middle}

	outer.SetNamespace("foo")

	for i, c := range []*minicli.Command{outer, middle, inner} {
		if c.Namespace != "foo" {
			t.Errorf("level %d namespace = %q, want %q", i, c.Namespace, "foo")
		}
	}
}

// TestConcurrentNamespaceResolution is the regression test. Before commands
// carried their own namespace, `namespace <name> <cmd>` resolved by setting the
// process-global active namespace, running, and setting it back -- while
// cmdProcessor ran every command in its own goroutine. Concurrent clients
// therefore read each other's namespace and executed against the wrong one,
// which minimega itself logs as "unexpected namespace ... when reverting to".
//
// Here many goroutines resolve tagged commands for distinct namespaces while
// another goroutine churns the active namespace underneath them. Every
// resolution must still return the namespace its command named.
func TestConcurrentNamespaceResolution(t *testing.T) {
	registerNamespace(t, DefaultNamespace)

	defer func() { _ = SetNamespace(DefaultNamespace) }()

	const (
		workers    = 8
		iterations = 200
	)

	names := make([]string, workers)

	for i := range names {
		names[i] = fmt.Sprintf("race-ns-%d", i)
		registerNamespace(t, names[i])
	}

	var (
		resolvers sync.WaitGroup
		churn     sync.WaitGroup
		done      = make(chan struct{})
	)

	// Churn the active namespace, standing in for unrelated concurrent clients.
	churn.Add(1)

	go func() {
		defer churn.Done()

		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
				_ = SetNamespace(names[i%len(names)])
			}
		}
	}()

	errs := make(chan error, workers*iterations)

	for _, name := range names {
		resolvers.Add(1)

		go func(name string) {
			defer resolvers.Done()

			cmd := &minicli.Command{Original: "vm info", Namespace: name}

			for range iterations {
				if got := resolveNamespace(cmd).Name; got != name {
					errs <- fmt.Errorf("resolved to %q, want %q", got, name)
					return
				}
			}
		}(name)
	}

	resolvers.Wait()
	close(done)
	churn.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}
