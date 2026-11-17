// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fcMountsFavor = `sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
cgroup2 /sys/fs/cgroup cgroup2 rw,nosuid,nodev,noexec,relatime,nsdelegate,favordynmods,memory_recursiveprot 0 0
`
	fcMountsNoFavor = `sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
cgroup2 /sys/fs/cgroup cgroup2 rw,nosuid,nodev,noexec,relatime,nsdelegate,memory_recursiveprot 0 0
`
)

func TestFCCgroup2Options(t *testing.T) {
	opts, ok := fcCgroup2Options(fcMountsFavor)
	if !ok || !strings.Contains(strings.Join(opts, ","), "favordynmods") {
		t.Errorf("favordynmods mount: got %v, %v", opts, ok)
	}

	// The mount at hostCgroupRoot wins over an earlier cgroup2 mount.
	mounts := "cgroup2 /other cgroup2 rw,favordynmods 0 0\n" + fcMountsNoFavor
	opts, ok = fcCgroup2Options(mounts)
	if !ok || strings.Contains(strings.Join(opts, ","), "favordynmods") {
		t.Errorf("mount at %v: got %v, %v", hostCgroupRoot, opts, ok)
	}

	// Only another cgroup2 mount.
	opts, ok = fcCgroup2Options("cgroup2 /other cgroup2 rw,nsdelegate 0 0\n")
	if !ok || len(opts) != 2 {
		t.Errorf("other cgroup2 mount: got %v, %v", opts, ok)
	}

	if _, ok := fcCgroup2Options("tmpfs /tmp tmpfs rw 0 0\ncgroup /sys/fs/cgroup/cpu cgroup rw,cpu 0 0\n"); ok {
		t.Error("no cgroup2 mount: found one")
	}
}

func TestFCTuningProblems(t *testing.T) {
	good := fcHostTuning{
		CgroupVersion: 2,
		Mounts:        fcMountsFavor,
		NofileOK:      true,
		NofileSoft:    1048576,
		THP:           "always [madvise] never\n",
		KSM:           "0\n",
		OOMScoreAdj:   "-1000\n",
		RootIsChild:   false,
		RootProcs:     "1\n2\n",
	}

	cases := []struct {
		name  string
		edit  func(*fcHostTuning)
		warn  string
		info  string
		clean bool
	}{
		{name: "good", edit: func(*fcHostTuning) {}, clean: true},
		{name: "no favordynmods", edit: func(h *fcHostTuning) { h.Mounts = fcMountsNoFavor }, warn: "favordynmods"},
		{name: "no favordynmods on v1", edit: func(h *fcHostTuning) { h.Mounts = fcMountsNoFavor; h.CgroupVersion = 1 }, clean: true},
		{name: "no cgroup2 mount", edit: func(h *fcHostTuning) { h.Mounts = "" }, clean: true},
		{name: "docker nofile", edit: func(h *fcHostTuning) { h.NofileSoft = 1024 }, warn: "--ulimit nofile=1048576:1048576"},
		{name: "nofile at limit", edit: func(h *fcHostTuning) { h.NofileSoft = fcMinNofile }, clean: true},
		{name: "nofile unknown", edit: func(h *fcHostTuning) { h.NofileOK = false; h.NofileSoft = 0 }, clean: true},
		{name: "thp always", edit: func(h *fcHostTuning) { h.THP = "[always] madvise never\n" }, warn: "madvise"},
		{name: "thp never", edit: func(h *fcHostTuning) { h.THP = "always madvise [never]\n" }, clean: true},
		{name: "ksm on", edit: func(h *fcHostTuning) { h.KSM = "1\n" }, warn: "KSM is on"},
		{name: "ksm unreadable", edit: func(h *fcHostTuning) { h.KSM = "" }, clean: true},
		{name: "oom score", edit: func(h *fcHostTuning) { h.OOMScoreAdj = "0\n" }, info: "oom_score_adj is 0"},
		{name: "oom unreadable", edit: func(h *fcHostTuning) { h.OOMScoreAdj = "" }, clean: true},
		{name: "container root with processes", edit: func(h *fcHostTuning) { h.RootIsChild = true }, warn: "child cgroup"},
		{name: "container root empty", edit: func(h *fcHostTuning) { h.RootIsChild = true; h.RootProcs = "\n" }, clean: true},
		{name: "container root on v1", edit: func(h *fcHostTuning) { h.RootIsChild = true; h.CgroupVersion = 1 }, clean: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := good
			c.edit(&h)

			warns, infos := fcTuningProblems(h)
			if c.clean {
				if len(warns) != 0 || len(infos) != 0 {
					t.Errorf("got warns %q, infos %q; want none", warns, infos)
				}
				return
			}

			if c.warn != "" && (len(warns) != 1 || !strings.Contains(warns[0], c.warn)) {
				t.Errorf("got warns %q; want one with %q", warns, c.warn)
			}
			if c.warn == "" && len(warns) != 0 {
				t.Errorf("got warns %q; want none", warns)
			}
			if c.info != "" && (len(infos) != 1 || !strings.Contains(infos[0], c.info)) {
				t.Errorf("got infos %q; want one with %q", infos, c.info)
			}
			if c.info == "" && len(infos) != 0 {
				t.Errorf("got infos %q; want none", infos)
			}
		})
	}
}

func TestFCReadHostTuningRoot(t *testing.T) {
	root := t.TempDir()

	h := fcReadHostTuning(root)
	if h.RootIsChild || h.RootProcs != "" {
		t.Errorf("empty root: got RootIsChild %v, RootProcs %q", h.RootIsChild, h.RootProcs)
	}

	for name, data := range map[string]string{"cgroup.type": "domain\n", "cgroup.procs": "1\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}

	h = fcReadHostTuning(root)
	if !h.RootIsChild || h.RootProcs != "1\n" {
		t.Errorf("child root: got RootIsChild %v, RootProcs %q", h.RootIsChild, h.RootProcs)
	}
}
