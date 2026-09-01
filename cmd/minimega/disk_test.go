// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"reflect"
	"testing"
)

func TestParseInjectPairs(t *testing.T) {
	cases := []struct {
		name    string
		in      []string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "single pair",
			in:   []string{"src:dst"},
			want: map[string]string{"dst": "src"},
		},
		{
			name: "multiple pairs",
			in:   []string{"src1:dst1", "src2:dst2"},
			want: map[string]string{"dst1": "src1", "dst2": "src2"},
		},
		{
			name:    "malformed - no colon",
			in:      []string{"srcdst"},
			wantErr: true,
		},
		{
			name:    "malformed - too many colons",
			in:      []string{"src:dst:extra"},
			wantErr: true,
		},
		{
			name:    "duplicate destination",
			in:      []string{"src1:dst", "src2:dst"},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseInjectPairs(c.in)

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("pairs = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseFiles(t *testing.T) {
	cases := []struct {
		name      string
		files     interface{}
		delete    bool
		wantPairs map[string]string
		wantPaths []string
		wantErr   bool
	}{
		{
			name:      "inject list arg",
			files:     []string{"src1:dst1", "src2:dst2"},
			wantPairs: map[string]string{"dst1": "src1", "dst2": "src2"},
		},
		{
			name:      "inject string arg (options case)",
			files:     "src:dst",
			wantPairs: map[string]string{"dst": "src"},
		},
		{
			name:      "delete single path from list arg",
			delete:    true,
			files:     []string{"foo.txt"},
			wantPaths: []string{"foo.txt"},
		},
		{
			name:      "delete comma-separated paths from list arg",
			delete:    true,
			files:     []string{"foo.txt,bar/baz.txt"},
			wantPaths: []string{"foo.txt", "bar/baz.txt"},
		},
		{
			name:      "delete single path from string arg",
			delete:    true,
			files:     "foo.txt",
			wantPaths: []string{"foo.txt"},
		},
		{
			name:      "delete comma-separated paths from string arg",
			delete:    true,
			files:     "foo.txt,bar/baz.txt",
			wantPaths: []string{"foo.txt", "bar/baz.txt"},
		},
		{
			name:    "malformed inject pair",
			files:   []string{"nodest"},
			wantErr: true,
		},
		{
			name:    "unknown type",
			files:   42,
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pairs, paths, err := parseFiles(c.files, c.delete)

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantPairs != nil && !reflect.DeepEqual(pairs, c.wantPairs) {
				t.Errorf("pairs = %v, want %v", pairs, c.wantPairs)
			}
			if c.wantPaths != nil && !reflect.DeepEqual(paths, c.wantPaths) {
				t.Errorf("paths = %v, want %v", paths, c.wantPaths)
			}
		})
	}
}

func TestWritableMountArgsNoOptions(t *testing.T) {
	candidates := writableMountArgs(nil)("/dev/nbd0p1")

	want := [][]string{
		{"mount", "-w", "/dev/nbd0p1"},
		{"mount", "-o", "ntfs-3g", "/dev/nbd0p1"},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Errorf("candidates = %v, want %v", candidates, want)
	}
}

func TestWritableMountArgsWithOptions(t *testing.T) {
	options := []string{"-t", "fat", "-o", "offset=100"}
	candidates := writableMountArgs(options)("/dev/nbd0p1")

	want := [][]string{
		{"mount", "-t", "fat", "-o", "offset=100", "/dev/nbd0p1"},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Errorf("candidates = %v, want %v", candidates, want)
	}
}

func TestReadOnlyMountArgsNoOptions(t *testing.T) {
	candidates := readOnlyMountArgs(nil)("/dev/nbd0p1")

	want := [][]string{
		{"mount", "-r", "/dev/nbd0p1"},
		{"mount", "-r", "-o", "noload", "/dev/nbd0p1"},
		{"mount", "-r", "-o", "norecovery", "/dev/nbd0p1"},
		{"mount", "-r", "-o", "ntfs-3g,force", "/dev/nbd0p1"},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Errorf("candidates = %v, want %v", candidates, want)
	}
}

func TestReadOnlyMountArgsWithOptions(t *testing.T) {
	options := []string{"-t", "fat", "-o", "offset=100"}
	candidates := readOnlyMountArgs(options)("/dev/nbd0p1")

	want := [][]string{
		{"mount", "-t", "fat", "-o", "offset=100", "/dev/nbd0p1"},
	}

	if !reflect.DeepEqual(candidates, want) {
		t.Errorf("candidates = %v, want %v", candidates, want)
	}
}
