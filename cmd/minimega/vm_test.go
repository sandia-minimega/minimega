package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A VM that errored before launch created the namespace alias must still be
// flushable: removed from the VM map, with its instance directory cleaned up.
func TestVMsFlushErroredBeforeAlias(t *testing.T) {
	vms := flushableVMs(t, 1)
	vm := vms.m[0].(*AndroidVM)

	if err := os.Remove(filepath.Join(*f_base, "namespaces", "ns", vm.UUID)); err != nil {
		t.Fatal(err)
	}

	if err := vms.Flush("0", nil); err != nil {
		t.Fatalf("flush failed: %v", err)
	}
	if len(vms.m) != 0 {
		t.Fatalf("VM still in map after flush")
	}
	if _, err := os.Stat(vm.instancePath); !os.IsNotExist(err) {
		t.Fatalf("instance path leaked: %v", err)
	}
}
