// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"slices"
	"testing"
)

// TestListingRecreatesDefaultNamespace covers `clear namespace minimega`: the
// default namespace stays active, so listing namespaces must still show it
// (miniweb relies on that listing to decide which namespaces exist).
func TestListingRecreatesDefaultNamespace(t *testing.T) {
	oldIOM := *f_iomBase
	*f_iomBase = t.TempDir()

	// simulate a cleared default namespace
	namespaceLock.Lock()
	old, hadDefault := namespaces[DefaultNamespace]
	delete(namespaces, DefaultNamespace)
	namespaceLock.Unlock()

	t.Cleanup(func() {
		namespaceLock.Lock()
		defer namespaceLock.Unlock()

		if ns := namespaces[DefaultNamespace]; ns != nil && ns != old && ns.ccServer != nil {
			ns.ccServer.Destroy()
		}

		delete(namespaces, DefaultNamespace)
		if hadDefault {
			namespaces[DefaultNamespace] = old
		}

		*f_iomBase = oldIOM
	})

	if names := ListNamespaces(); !slices.Contains(names, DefaultNamespace) {
		t.Errorf("ListNamespaces() = %v, missing %v", names, DefaultNamespace)
	}

	found := false
	for _, info := range InfoNamespaces() {
		if info.Name == DefaultNamespace {
			found = true
		}
	}
	if !found {
		t.Errorf("InfoNamespaces() missing %v", DefaultNamespace)
	}
}
