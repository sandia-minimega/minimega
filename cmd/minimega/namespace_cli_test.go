package main

import (
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// After del-bridge succeeds the bridge must no longer be tracked, otherwise
// `clear namespace` re-runs del-bridge for it and fails on the missing bridge.
func TestNamespaceDelBridgeUntracksBridge(t *testing.T) {
	ns := &Namespace{
		Name:    "t",
		Hosts:   map[string]bool{},
		Bridges: map[string]uint32{"br0": 1},
	}

	c := minicli.MustCompile("ns del-bridge br0")
	if err := cliNamespaceDelBridge(ns, c, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}

	if _, ok := ns.Bridges["br0"]; ok {
		t.Fatal("bridge still tracked after del-bridge")
	}

	if err := cliNamespaceDelBridge(ns, c, &minicli.Response{}); err == nil {
		t.Fatal("second del-bridge should fail: bridge not associated")
	}
}
