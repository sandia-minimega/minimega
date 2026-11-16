package main

import (
	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func TestCLILogRingInvalidSize(t *testing.T) {
	for _, size := range []string{"0", "-1"} {
		oldRing := logRing
		t.Cleanup(func() {
			log.DelLogger("ring")
			logRing = oldRing
		})

		c := &minicli.Command{StringArgs: map[string]string{"size": size}}
		if err := cliLogRing(nil, c, &minicli.Response{}); err == nil {
			t.Errorf("log ring %s: expected error", size)
		}

		// later log calls must not panic
		log.Info("after log ring %s", size)
	}
}

func TestCLILogMeshSelfKeepsExisting(t *testing.T) {
	oldHost, oldNode, oldMesh := hostname, meshageNode, logMeshNode
	t.Cleanup(func() {
		log.DelLogger("meshage")
		hostname, meshageNode, logMeshNode = oldHost, oldNode, oldMesh
	})

	hostname = "self"
	meshageNode, _ = meshage.NewNode(hostname, "", 1, nil, 19999, "test")

	if err := setupMeshageLogging("head"); err != nil {
		t.Fatal(err)
	}

	c := &minicli.Command{StringArgs: map[string]string{"node": hostname}}
	if err := cliLogMesh(nil, c, &minicli.Response{}); err == nil {
		t.Fatal("expected error for self as destination")
	}

	if logMeshNode != "head" {
		t.Errorf("logMeshNode = %q, want head", logMeshNode)
	}
	if _, err := log.GetLevel("meshage"); err != nil {
		t.Errorf("existing meshage logger was removed: %v", err)
	}
}
