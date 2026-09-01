// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import "testing"

func TestSplitImagePartition(t *testing.T) {
	cases := []struct {
		name          string
		in            string
		wantImage     string
		wantPartition string
		wantErr       bool
	}{
		{
			name:      "no partition",
			in:        "/tmp/minimega/files/foo.qcow2",
			wantImage: "/tmp/minimega/files/foo.qcow2",
		},
		{
			name:          "with partition",
			in:            "/tmp/minimega/files/foo.qcow2:2",
			wantImage:     "/tmp/minimega/files/foo.qcow2",
			wantPartition: "2",
		},
		{
			name:          "none partition",
			in:            "/tmp/minimega/files/foo.qcow2:none",
			wantImage:     "/tmp/minimega/files/foo.qcow2",
			wantPartition: "none",
		},
		{
			name:    "too many colons",
			in:      "/tmp/minimega/files/foo.qcow2:2:3",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			image, partition, err := splitImagePartition(c.in)

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if image != c.wantImage {
				t.Errorf("image = %q, want %q", image, c.wantImage)
			}
			if partition != c.wantPartition {
				t.Errorf("partition = %q, want %q", partition, c.wantPartition)
			}
		})
	}
}
