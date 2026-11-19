// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"slices"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		version string
		want    []int
		wantErr bool
	}{
		{version: "6.2.0", want: []int{6, 2, 0}},
		{version: "8.2.2", want: []int{8, 2, 2}},
		{version: "11.1.3", want: []int{11, 1, 3}},
		{version: "7.2.0-rc1", want: []int{7, 2, 0}},
		{version: "6.8.0-45-generic", want: []int{6, 8, 0}},
		{version: "6.8.0-45-generic\n", want: []int{6, 8, 0}},
		{version: "2.90rc1.3", want: []int{2, 90}},
		{version: "abc", wantErr: true},
		{version: "1.x", wantErr: true},
		{version: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got, err := parseVersion("test", tt.version)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
