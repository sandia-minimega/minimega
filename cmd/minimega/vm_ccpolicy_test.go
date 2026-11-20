// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

func TestBaseVMCCPolicy(t *testing.T) {
	for _, tt := range []struct {
		typ    VMType
		config string
		want   string
	}{
		{FIRECRACKER, "", ron.CCPolicyStrict},
		{KVM, "", ron.CCPolicyLegacy},
		{CONTAINER, "", ron.CCPolicyLegacy},
		{ANDROID, "", ron.CCPolicyLegacy},
		{FIRECRACKER, ron.CCPolicyLegacy, ron.CCPolicyLegacy},
		{KVM, ron.CCPolicyStrict, ron.CCPolicyStrict},
	} {
		vm := &BaseVM{Type: tt.typ}
		vm.BaseConfig.CCPolicy = tt.config

		if got := vm.CCPolicy(); got != tt.want {
			t.Errorf("%v with cc-policy %q: CCPolicy() = %v, want %v", tt.typ, tt.config, got, tt.want)
		}
	}
}
