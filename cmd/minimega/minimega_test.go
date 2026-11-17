// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidVMName(t *testing.T) {
	label63 := strings.Repeat("a", 63)
	// three 63 character labels, one of 61 and three dots make 253 characters
	name253 := strings.Join([]string{label63, label63, label63, label63[:61]}, ".")
	// 127 one character labels and 126 dots make 253 characters
	dots253 := strings.TrimSuffix(strings.Repeat("a.", 127), ".")

	if len(name253) != 253 || len(dots253) != 253 {
		t.Fatalf("test names have %v and %v characters, want 253", len(name253), len(dots253))
	}

	tests := []struct {
		name  string
		valid bool
	}{
		// length: 2 to 253 characters
		{"", false},
		{"a", false},
		{"0", false},
		{"-", false},
		{".", false},
		{"ab", true},
		{"a0", true},
		{"0a", true},
		{"a.b", true},
		{name253, true},
		{dots253, true},
		{name253 + "a", false},
		{dots253 + "a", false},
		{dots253 + ".a", false},

		// labels: 1 to 63 characters
		{label63, true},
		{label63 + "." + label63, true},
		{"a." + label63, true},
		{label63 + "a", false},
		{"web." + label63 + "a", false},
		{label63 + "a.web", false},

		// hyphens: inside a label only
		{"web-1", true},
		{"web--1", true},
		{"a-b", true},
		{"-web", false},
		{"web-", false},
		{"-a", false},
		{"a-", false},
		{"--", false},
		{"web.-lan", false},
		{"web-.lan", false},

		// dots: separate labels that are not empty
		{"web1.example.com", true},
		{"host.leaf-1.net", true},
		{"..", false},
		{".web", false},
		{"web.", false},
		{".a", false},
		{"a.", false},
		{"web..lan", false},
		{"web.example.com.", false},

		// letter case and digits
		{"Web-1", true},
		{"WEB", true},
		{"A.B", true},
		{"wEb.LaN", true},
		{"1web", true},
		{"12", true},
		{"10.0.0.1", true},
		{"0.0", true},

		// other characters
		{"web_1", false},
		{"web_server", false},
		{"_web", false},
		{"web_", false},
		{"__", false},
		{"web 1", false},
		{" web", false},
		{"web ", false},
		{"web\t1", false},
		{"web\n", false},
		{"web\x00", false},
		{"web/1", false},
		{"/web", false},
		{"web/", false},
		{`web\1`, false},
		{`\web`, false},
		{`web\`, false},
		{"web:1", false},
		{"web,1", false},
		{"web*", false},
		{"web[1]", false},

		// non-ASCII text, also when its byte length is in range
		{"wéb", false},
		{"é", false},
		{"w\u0435b", false},           // Cyrillic small letter ie
		{"\uff57\uff45\uff42", false}, // full-width web
		{"web\u00a0", false},          // no-break space
		{"xn--wb-bja", true},          // the ASCII (punycode) form of wéb
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.name), func(t *testing.T) {
			err := validVMName(tt.name)
			if tt.valid {
				if err != nil {
					t.Errorf("validVMName(%q) = %v, want nil", tt.name, err)
				}
				return
			}

			if err == nil {
				t.Fatalf("validVMName(%q) = nil, want error", tt.name)
			}

			// the error must name the VM and tell the rule
			if !strings.Contains(err.Error(), "`"+tt.name+"`") || !strings.Contains(err.Error(), "RFC 1123") {
				t.Errorf("validVMName(%q) error does not tell the name and the rule: %v", tt.name, err)
			}

			if n := len(tt.name); (n < 2 || n > 253) && !strings.Contains(err.Error(), fmt.Sprintf("length is %v", n)) {
				t.Errorf("validVMName(%q) error does not tell the length: %v", tt.name, err)
			}
		})
	}
}

// queueTestNamespace registers a namespace on this host with the existing VMs
// and the queued VM names.
func queueTestNamespace(t *testing.T, existing []string, queued []string) *Namespace {
	t.Helper()

	ns := &Namespace{
		Name:  "vm-name-case",
		Hosts: map[string]bool{hostname: true},
		VMs:   VMs{m: map[int]VM{}},
	}

	for i, name := range existing {
		ns.VMs.m[i] = &KvmVM{BaseVM: &BaseVM{ID: i, Name: name, Type: KVM, Host: hostname}}
	}

	if len(queued) > 0 {
		ns.queue = []*QueuedVMs{{Names: queued, VMType: KVM}}
	}

	namespaceLock.Lock()
	original, ok := namespaces[ns.Name]
	namespaces[ns.Name] = ns
	namespaceLock.Unlock()

	t.Cleanup(func() {
		namespaceLock.Lock()
		defer namespaceLock.Unlock()

		if ok {
			namespaces[ns.Name] = original
		} else {
			delete(namespaces, ns.Name)
		}
	})

	return ns
}

func TestQueueVMNameLetterCase(t *testing.T) {
	tests := []struct {
		desc     string
		existing []string
		queued   []string
		launch   string
		// the error must contain each of these, or be nil when there are none
		want []string
	}{
		{"existing vm", []string{"web1"}, nil, "WEB1", []string{"`WEB1`", "existing vm `web1`", "RFC 4343"}},
		{"existing vm in a list", []string{"Web1"}, nil, "db1,web1", []string{"`web1`", "existing vm `Web1`"}},
		{"queued vm", nil, []string{"web1"}, "WEB1", []string{"`WEB1`", "queued vm `web1`", "RFC 4343"}},
		{"queued vm in a range", nil, []string{"WEB2"}, "web[1-3]", []string{"`web2`", "queued vm `WEB2`"}},
		{"same launch list", nil, nil, "web1,WEB1", []string{"`web1` and `WEB1`", "RFC 4343"}},
		{"same launch list, dotted", nil, nil, "web1.lan,WEB1.LAN", []string{"`web1.lan` and `WEB1.LAN`"}},
		{"same name exists", []string{"web1"}, nil, "web1", []string{"vm already exists with name `web1`"}},
		{"same name queued", nil, []string{"web1"}, "web1", []string{"vm already queued with name `web1`"}},
		{"same name twice", nil, nil, "web1,web1", []string{"`web1` is specified twice"}},
		{"other existing name", []string{"web1"}, nil, "WEB2", nil},
		{"other queued name", nil, []string{"web1"}, "web-1,web11,WEB1X", nil},
		{"other names in one list", nil, nil, "web1,WEB2,Web3", nil},
		{"unnamed vms", []string{"vm-0"}, nil, "2", nil},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ns := queueTestNamespace(t, tt.existing, tt.queued)

			err := ns.Queue(tt.launch, KVM, VMConfig{})
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatalf("Queue(%q) = %v, want nil", tt.launch, err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Queue(%q) = nil, want error", tt.launch)
			}

			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Queue(%q) error does not contain %q: %v", tt.launch, want, err)
				}
			}

			if want := min(len(tt.queued), 1); len(ns.queued()) != want {
				t.Errorf("Queue(%q) changed the queue after an error", tt.launch)
			}
		})
	}
}

func TestBaseVMConflictsLetterCase(t *testing.T) {
	existing := &BaseVM{Name: "web1", BaseConfig: BaseConfig{UUID: "uuid-1"}}

	tests := []struct {
		name string
		want string
	}{
		{"WEB1", "duplicate VM name: WEB1 conflicts with web1"},
		{"Web1", "duplicate VM name: Web1 conflicts with web1"},
		{"web1", "duplicate VM name: web1"},
		{"web2", ""},
		{"WEB2", ""},
		{"web1x", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &BaseVM{Name: tt.name, BaseConfig: BaseConfig{UUID: "uuid-2"}}

			err := existing.conflicts(vm)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("conflicts(%q) = %v, want nil", tt.name, err)
				}
				return
			}

			if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Fatalf("conflicts(%q) = %v, want %q", tt.name, err, tt.want)
			}
		})
	}
}

func TestVMNameKey(t *testing.T) {
	tests := []struct {
		a, b string
		same bool
	}{
		{"web1", "WEB1", true},
		{"Web-1.Example.COM", "web-1.example.com", true},
		{"web1", "web2", false},
		{"web1", "web1.", false},
		// only ASCII letters fold: U+212A KELVIN SIGN is not "k"
		{"\u212aube", "kube", false},
		{"wÉb", "wéb", false},
		// bytes that are not UTF-8 stay different
		{"web\xff", "web\xfe", false},
	}

	for _, tt := range tests {
		if got := vmNameKey(tt.a) == vmNameKey(tt.b); got != tt.same {
			t.Errorf("vmNameKey(%q) == vmNameKey(%q) is %v, want %v", tt.a, tt.b, got, tt.same)
		}
	}
}
