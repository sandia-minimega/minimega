package main

import (
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// A remote that cannot run a command must still answer, otherwise the sender
// waits forever (default mesh timeout) holding meshageCommandLock.
func TestMeshageExecRepliesOnInvalid(t *testing.T) {
	cases := map[string]minicli.Command{
		"uncompilable":  {Original: "this is not a command"},
		"bad namespace": {Original: "version", Namespace: "bad name!"},
	}

	for name, c := range cases {
		resp := meshageExec(meshageCommand{Command: c, TID: 42})
		if resp.TID != 42 || resp.Error == "" {
			t.Errorf("%s: want error response for TID 42, got %#v", name, resp)
		}
	}
}
