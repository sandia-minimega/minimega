package rbac

import (
	"testing"
)

func TestGlobalAdmin(t *testing.T) {
	policies := CreateBasePoliciesForRole("Global Admin")
	role := NewRole("Global Admin", policies...)

	if !role.Allowed("experiments/start", "update") {
		t.Fatal("expected global admin to be able to start experiment")
	}
}

func TestVMViewer(t *testing.T) {
	policies := CreateBasePoliciesForRole("VM Viewer")
	policies.AddResourceNames("foo_*_sucka", "!foo_fish_sucka")

	role := NewRole("VM Viewer", policies...)

	if role.Allowed("experiments/start", "update") {
		t.Fatal("didn't expect VM viewer to be able to start experiment")
	}

	if !role.Allowed("vms/vnc", "get", "foo_bar_sucka") {
		t.Fatal("expected VM viewer to be able to access VNC for foo_bar_sucka")
	}

	if role.Allowed("vms/vnc", "get", "foo_fish_sucka") {
		t.Fatal("expected VM viewer not to be able to access VNC for foo_fish_sucka")
	}
}
