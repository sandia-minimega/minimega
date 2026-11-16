package main

import (
	"strconv"
	"testing"
)

// Namespace.schedule adds resp.Response to the launched count, so a clean
// remote launch of N VMs must report N, as the local launch path does.
func TestMeshageLaunchResponseCountsVMs(t *testing.T) {
	queued := &QueuedVMs{Names: []string{"a", "b", "c"}}

	resp := meshageLaunchResponse("remote", queued, meshageVMResponse{})
	if n, _ := strconv.Atoi(resp.Response); n != 3 || resp.Error != "" {
		t.Errorf("want 3 launched and no error, got %#v", resp)
	}
}
