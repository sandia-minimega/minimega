package main

import (
	"fmt"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
)

func TestParseFilesDeleteList(t *testing.T) {
	tests := []struct {
		in   interface{}
		want []string
	}{
		{[]string{"foo.txt", "bar.txt"}, []string{"foo.txt", "bar.txt"}},
		{[]string{"a", "b,c"}, []string{"a", "b", "c"}},
		{[]string{"a,b"}, []string{"a", "b"}},
		{[]string{"a"}, []string{"a"}},
		{"a,b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		_, got, err := parseFiles(tt.in, true)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseFiles(%v) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
}

func TestCLIDiskSnapshotNoDstDoesNotLeak(t *testing.T) {
	old := *f_iomBase
	*f_iomBase = t.TempDir()
	defer func() { *f_iomBase = old }()

	nfds := func() int {
		ents, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Skip("needs /proc")
		}
		return len(ents)
	}

	c := &minicli.Command{StringArgs: map[string]string{"image": "missing.qcow2"}}
	before := nfds()
	for i := 0; i < 5; i++ {
		// the source image does not exist so the snapshot fails
		if err := cliDiskSnapshot(nil, c, &minicli.Response{}); err == nil {
			t.Fatal("expected error")
		}
	}
	if after := nfds(); after != before {
		t.Errorf("leaked %d file descriptors", after-before)
	}
	if ents, _ := os.ReadDir(*f_iomBase); len(ents) != 0 {
		t.Errorf("failed snapshot left %d temp files behind", len(ents))
	}
}

// fake qemu-img printing a two-image backing chain.
func TestCLIDiskInfoRecursiveInUsePerImage(t *testing.T) {
	dir := t.TempDir()
	top, base := filepath.Join(dir, "top.qcow2"), filepath.Join(dir, "base.qcow2")
	for _, p := range []string{top, base} {
		if err := os.WriteFile(p, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// only the backing image is locked
	f, err := os.Open(base)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		t.Fatal(err)
	}

	chain := fmt.Sprintf(`[{"filename":%q,"format":"qcow2","virtual-size":1,"actual-size":1,"backing-filename":"base.qcow2"},`+
		`{"filename":%q,"format":"qcow2","virtual-size":1,"actual-size":1}]`, top, base)
	bin := t.TempDir()
	script := "#!/bin/sh\ncat <<'EOF'\n" + chain + "\nEOF\n"
	if err := os.WriteFile(filepath.Join(bin, "qemu-img"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))

	c := &minicli.Command{
		StringArgs: map[string]string{"image": top},
		BoolArgs:   map[string]bool{"recursive": true},
	}
	resp := &minicli.Response{}
	if err := cliDiskInfo(nil, c, resp); err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	for _, row := range resp.Tabular {
		got[row[0]] = row[5]
	}
	if got[top] != "false" || got[base] != "true" {
		t.Errorf("inuse by image = %v; want top false, base true", got)
	}
}
