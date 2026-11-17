// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestParseDiskConfig(t *testing.T) {
	tests := []struct {
		spec string
		want DiskConfig
	}{
		{"/img/a.qcow2", DiskConfig{Path: "/img/a.qcow2"}},
		{"/img/a.qcow2,virtio", DiskConfig{Path: "/img/a.qcow2", Interface: "virtio"}},
		{"/img/a.qcow2,unsafe", DiskConfig{Path: "/img/a.qcow2", Cache: "unsafe"}},
		{"/img/a.qcow2,ide,writeback", DiskConfig{Path: "/img/a.qcow2", Interface: "ide", Cache: "writeback"}},
		{"/img/root.squashfs,ro", DiskConfig{Path: "/img/root.squashfs", ReadOnly: true}},
		{"/img/root.raw,virtio,ro", DiskConfig{Path: "/img/root.raw", Interface: "virtio", ReadOnly: true}},
		{"/img/root.raw,unsafe,ro", DiskConfig{Path: "/img/root.raw", Cache: "unsafe", ReadOnly: true}},
		{"/img/root.raw,virtio,writeback,ro", DiskConfig{Path: "/img/root.raw", Interface: "virtio", Cache: "writeback", ReadOnly: true}},
	}

	for _, test := range tests {
		got, err := ParseDiskConfig(test.spec, true)
		if err != nil {
			t.Errorf("ParseDiskConfig(%q) = %v", test.spec, err)
			continue
		}
		if *got != test.want {
			t.Errorf("ParseDiskConfig(%q) = %+v, want %+v", test.spec, *got, test.want)
		}
		if s := got.String(); s != test.spec {
			t.Errorf("String() of %q = %q", test.spec, s)
		}

		again, err := ParseDiskConfig(got.String(), false)
		if err != nil || *again != *got {
			t.Errorf("ParseDiskConfig(String()) of %q = %+v, %v, want %+v", test.spec, again, err, *got)
		}
	}

	for _, spec := range []string{
		"/img/a.raw,ro,virtio",
		"/img/a.raw,ro,ro",
		"/img/a.raw,unsafe,virtio",
		"/img/a.raw,bogus",
		"/img/a.raw,bogus,ro",
		"/img/a.raw,virtio,unsafe,writeback",
		"/img/a.raw,virtio,unsafe,ro,ro",
	} {
		if got, err := ParseDiskConfig(spec, false); err == nil {
			t.Errorf("ParseDiskConfig(%q) = %+v, want error", spec, *got)
		}
	}
}

func TestDiskConfigsWriteConfig(t *testing.T) {
	disks := DiskConfigs{
		{Path: "/img/a.qcow2", Interface: "virtio"},
		{Path: "/img/root.squashfs", ReadOnly: true},
	}

	var buf bytes.Buffer
	if err := disks.WriteConfig(&buf); err != nil {
		t.Fatal(err)
	}
	if want := "vm config disks /img/a.qcow2,virtio /img/root.squashfs,ro\n"; buf.String() != want {
		t.Errorf("WriteConfig() = %q, want %q", buf.String(), want)
	}

	var kvm KVMConfig
	if err := kvm.ReadFieldConfig(strings.NewReader(buf.String()), "disks", DefaultNamespace, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kvm.Disks, disks) {
		t.Errorf("ReadFieldConfig() = %+v, want %+v", kvm.Disks, disks)
	}

	if got, err := kvm.Info("disks"); err != nil || got != "/img/a.qcow2,virtio /img/root.squashfs,ro" {
		t.Errorf("Info(disks) = %q, %v", got, err)
	}
}
