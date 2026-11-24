// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
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

// queueAndCheck runs Queue on ns with launch. It checks that the error
// contains wantErr, or that there is no error when wantErr is empty, and that
// only a call with no error adds to the queue. It returns the warnings.
func queueAndCheck(t *testing.T, ns *Namespace, launch, wantErr string) []string {
	t.Helper()

	before := len(ns.queued())

	warnings, err := ns.Queue(launch, KVM, VMConfig{})
	if wantErr == "" {
		if err != nil {
			t.Fatalf("Queue(%q) = %v, want nil", launch, err)
		}
		if len(ns.queued()) != before+1 {
			t.Errorf("Queue(%q) did not add to the queue", launch)
		}
		return warnings
	}

	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("Queue(%q) = %v, want an error with %q", launch, err, wantErr)
	}
	if warnings != nil {
		t.Errorf("Queue(%q) returned warnings %q with an error", launch, warnings)
	}
	if len(ns.queued()) != before {
		t.Errorf("Queue(%q) changed the queue after an error", launch)
	}

	return nil
}

// checkWarnings checks that there are count warnings, that each warning tells
// the release that will reject the name, and that the warnings contain each
// string in want.
func checkWarnings(t *testing.T, launch string, warnings []string, count int, want ...string) {
	t.Helper()

	if len(warnings) != count {
		t.Errorf("Queue(%q) warnings = %q, want %v warnings", launch, warnings, count)
	}

	for _, w := range warnings {
		if !strings.HasSuffix(w, "; "+futureRejection) {
			t.Errorf("Queue(%q) warning does not end with %q: %q", launch, futureRejection, w)
		}
	}

	all := strings.Join(warnings, "\n")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Errorf("Queue(%q) warnings do not contain %q: %q", launch, w, warnings)
		}
	}
}

// TestQueueVMNameLetterCase checks that Queue refuses an equal VM name and
// warns about a VM name that differs only in letter case.
func TestQueueVMNameLetterCase(t *testing.T) {
	tests := []struct {
		desc     string
		existing []string
		queued   []string
		launch   string
		wantErr  string
		// the number of warnings, and strings that the warnings must contain
		count int
		want  []string
	}{
		{"existing vm", []string{"web1"}, nil, "WEB1", "", 1, []string{"`WEB1`", "existing vm `web1`", "RFC 4343"}},
		{"existing vm in a list", []string{"Web1"}, nil, "db1,web1", "", 1, []string{"`web1`", "existing vm `Web1`"}},
		{"queued vm", nil, []string{"web1"}, "WEB1", "", 1, []string{"`WEB1`", "queued vm `web1`", "RFC 4343"}},
		{"queued vm in a range", nil, []string{"WEB2"}, "web[1-3]", "", 1, []string{"`web2`", "queued vm `WEB2`"}},
		{"existing and queued vm", []string{"web1"}, []string{"Web1"}, "WEB1", "", 2, []string{"existing vm `web1`", "queued vm `Web1`"}},
		{"same launch list", nil, nil, "web1,WEB1", "", 1, []string{"`web1` and `WEB1`", "RFC 4343"}},
		{"same launch list, three names", nil, nil, "web1,WEB1,Web1", "", 3, []string{"`web1` and `WEB1`", "`web1` and `Web1`", "`WEB1` and `Web1`"}},
		{"same launch list, dotted", nil, nil, "web1.lan,WEB1.LAN", "", 1, []string{"`web1.lan` and `WEB1.LAN`"}},
		{"same name exists", []string{"web1"}, nil, "web1", "vm already exists with name `web1`", 0, nil},
		{"same name queued", nil, []string{"web1"}, "web1", "vm already queued with name `web1`", 0, nil},
		{"same name twice", nil, nil, "web1,web1", "`web1` is specified twice", 0, nil},
		{"same name twice after a case-only name", nil, nil, "web1,WEB1,web1", "`web1` is specified twice", 0, nil},
		// both names have one key, so Queue must keep both
		{"same name exists with a case-only name", []string{"web1", "WEB1"}, nil, "web1", "vm already exists with name `web1`", 0, nil},
		{"case-only name exists with the same name", []string{"web1", "WEB1"}, nil, "WEB1", "vm already exists with name `WEB1`", 0, nil},
		{"same name queued with a case-only name", nil, []string{"web1", "WEB1"}, "web1", "vm already queued with name `web1`", 0, nil},
		{"case-only name queued with the same name", nil, []string{"web1", "WEB1"}, "WEB1", "vm already queued with name `WEB1`", 0, nil},
		{"other existing name", []string{"web1"}, nil, "WEB2", "", 0, nil},
		{"other queued name", nil, []string{"web1"}, "web-1,web11,WEB1X", "", 0, nil},
		{"other names in one list", nil, nil, "web1,WEB2,Web3", "", 0, nil},
		{"unnamed vms", []string{"vm-0"}, nil, "2", "", 0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ns := queueTestNamespace(t, tt.existing, tt.queued)

			warnings := queueAndCheck(t, ns, tt.launch, tt.wantErr)
			checkWarnings(t, tt.launch, warnings, tt.count, tt.want...)
		})
	}
}

// TestQueueVMNameRules checks that Queue refuses a VM name with characters
// that earlier releases refused, and warns about another name that is not a
// valid host name.
func TestQueueVMNameRules(t *testing.T) {
	label63 := strings.Repeat("a", 63)

	tests := []struct {
		launch  string
		wantErr string
		count   int
		want    []string
	}{
		// earlier releases refused these characters
		{"web/1", vmNameRule, 0, nil},
		{`web\1`, vmNameRule, 0, nil},
		{"web 1", vmNameRule, 0, nil},
		{"w\u00e9b", vmNameRule, 0, nil},
		{"web-1,web/2", "`web/2`", 0, nil},
		// earlier releases accepted these names
		{"web_1", "", 1, []string{"`web_1`", vmNameRule}},
		{"a", "", 1, []string{"`a`", "length is 1"}},
		{"-web", "", 1, []string{"`-web`"}},
		{"web-", "", 1, []string{"`web-`"}},
		{".web", "", 1, []string{"`.web`"}},
		{"web.", "", 1, []string{"`web.`"}},
		{"web..lan", "", 1, []string{"`web..lan`"}},
		{label63 + "a", "", 1, []string{"`" + label63 + "a`"}},
		{"web-1,web_[1-2]", "", 2, []string{"`web_1`", "`web_2`"}},
		// valid host names
		{"web-1", "", 0, nil},
		{"a.b", "", 0, nil},
		{label63, "", 0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.launch, func(t *testing.T) {
			ns := queueTestNamespace(t, nil, nil)

			warnings := queueAndCheck(t, ns, tt.launch, tt.wantErr)
			checkWarnings(t, tt.launch, warnings, tt.count, tt.want...)
		})
	}
}

// TestVMLaunchResponseWarnings checks that "vm launch" puts the warnings in
// its response.
func TestVMLaunchResponseWarnings(t *testing.T) {
	ns := queueTestNamespace(t, []string{"web1"}, nil)
	ns.QueueVMs = true

	resp := &minicli.Response{}
	if err := cliVMLaunch(ns, minicli.MustCompile("vm launch kvm WEB1,web_2"), resp); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(resp.Response, "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "existing vm `web1`") || !strings.Contains(lines[1], "`web_2`") {
		t.Errorf("response = %q, want the two warnings", resp.Response)
	}

	resp = &minicli.Response{}
	if err := cliVMLaunch(ns, minicli.MustCompile("vm launch kvm web-3"), resp); err != nil {
		t.Fatal(err)
	}
	if resp.Response != "" {
		t.Errorf("response = %q, want none", resp.Response)
	}
}

// TestBaseVMConflictsLetterCase checks that conflicts refuses an equal VM name
// and accepts a name that differs only in letter case.
func TestBaseVMConflictsLetterCase(t *testing.T) {
	existing := &BaseVM{Name: "web1", BaseConfig: BaseConfig{UUID: "uuid-1"}}

	tests := []struct {
		name string
		want string
	}{
		{"web1", "duplicate VM name: web1"},
		{"WEB1", ""},
		{"Web1", ""},
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
