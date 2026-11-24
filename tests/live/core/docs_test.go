// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build live

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	"github.com/sandia-minimega/minimega/v2/tests/live/harness"
)

// docsScript is one example script of the documentation that the live guest
// image can run as written.
type docsScript struct {
	// path below doc/content
	path string
	// namespace to run the script in, for a script that names none
	namespace string
	// namespaces to clear when the script ends
	clear []string
	// text that the output of the script must contain
	want []string
	// smallest number of pings in the output that got all replies
	pings int
	// check of the experiment that the script leaves, for a script that
	// prints nothing to compare
	check func(t *testing.T)
	// commands to run when the script ends, before the namespaces go
	after []string
}

// docsPingOK and docsPingLost are the summaries of a ping with all replies
// and of a ping with none.
const (
	docsPingOK   = ", 0% packet loss"
	docsPingLost = ", 100% packet loss"
)

// docsScripts lists the scripts in the order of the course. The other
// chapters need cgroup v1 (17), the Android SDK (18), swtpm (19), or a
// second host (20).
var docsScripts = func() []docsScript {
	chapters := []docsScript{
		{path: "02-images", want: []string{"client-test | RUNNING | true", "router-test | RUNNING | true"}},
		{path: "03-router-sandwich", want: []string{"router | RUNNING | [net_left"}},
		{path: "04-vms", want: []string{"vm-left | PAUSED", "vm-right | QUIT", `spare2 | RUNNING | {"role":"spare"}`}},
		{path: "05-miniweb", want: []string{"vm-right | RUNNING"}},
		{path: "06-networking", pings: 1, want: []string{"vm-right | [disconnected]", "vm-left | [0: loss 25]"}},
		{path: "07-routers", pings: 4, want: []string{"right.sandwich (10.0.1.10)", docsPingLost}},
		{path: "08-cc", pings: 1, want: []string{"10.0.1.1:22 | pass", "hello from cc", "vm-left | sleep 600"}},
		{path: "09-save-replay-cleanup", want: []string{"vm-left | completed", "saved/vm-left.state"}},
		{path: "11-traffic", want: []string{"<h1>protonuke</h1>", "protonuke -serve -http -https -smtp"}},
		{path: "12-capture", pings: 1, want: []string{"pcap | vm-left:0", "10.0.0.10:0", "ICMP echo request, id", "seq 0,"}},
		{path: "13-vnc", want: []string{"KeyEvent,true,Return", "vm-right | playback kb"}},
		{path: "14-runtime-changes", pings: 3, want: []string{docsPingLost, "[0: loss 25 delay 100ms]", "vm-right | /tmp/minimega/files/lab.iso", "usb.img"}},
		{path: "15-plumbing", want: []string{"the sandwich says hello", "sandwich//tagged | round-robin"}},
		{path: "16-namespaces", pings: 1, want: []string{"vm-left | RUNNING | [net_left"}},
	}

	for i := range chapters {
		chapters[i].path = "training/miniclass/scripts/" + chapters[i].path + ".mm"
		chapters[i].clear = []string{"sandwich", "sandwich2"}
	}

	return append(chapters,
		docsScript{
			path:      "articles/quickstart/quickstart.mm",
			namespace: "docs-quickstart",
			clear:     []string{"docs-quickstart"},
			// both VMs get a lease from dnsmasq on the host
			check: docsVMs("docs-quickstart", 2),
			// "clear namespace" does not stop dnsmasq. This command stops
			// each dnsmasq of the host, and the core topology starts none.
			after: []string{"dnsmasq kill all"},
		},
		docsScript{
			path:      "articles/capture/mirror.mm",
			namespace: "docs-mirror",
			clear:     []string{"docs-mirror"},
			// the agents of A and B run the two commands of the script
			check: docsCommands("docs-mirror", 2),
		},
	)
}()

// docsCommands returns a check that waits until n cc commands of the
// namespace have a response.
func docsCommands(ns string, n int) func(*testing.T) {
	return func(t *testing.T) {
		harness.Eventually(t, 2*time.Minute, 3*time.Second, func() error {
			resps, err := harness.Raw(".columns id,responses namespace " + ns + " cc commands")
			if err != nil {
				return err
			}

			var got int
			for _, row := range harness.ToRows(resps) {
				if row["responses"] == "1" {
					got++
				}
			}
			if got != n {
				return fmt.Errorf("%d of %d cc commands in namespace %v have a response", got, n, ns)
			}

			return nil
		})
	}
}

// docsVMs returns a check that waits until n VMs of the namespace have a
// connected agent and an address in 10.0.0.0/24.
func docsVMs(ns string, n int) func(*testing.T) {
	return func(t *testing.T) {
		harness.Eventually(t, 2*time.Minute, 3*time.Second, func() error {
			resps, err := harness.Raw(".columns name,cc_active,ip namespace " + ns + " vm info")
			if err != nil {
				return err
			}

			var got int
			for _, row := range harness.ToRows(resps) {
				if row["cc_active"] == "true" && strings.HasPrefix(row["ip"], "[10.0.0.") {
					got++
				}
			}
			if got != n {
				return fmt.Errorf("%d of %d VMs in namespace %v have an agent and an address", got, n, ns)
			}

			return nil
		})
	}
}

// docsBudget is the time that the scripts get in total. A script waits up to
// five minutes for its VMs, so a broken guest image can use all of the job.
const docsBudget = 30 * time.Minute

// testDocsScripts runs example scripts of the documentation with "read", as a
// reader does. It fails on each command that returns an error, and when the
// output does not show what the chapter demonstrates. The scripts
// name the images of the course, miniccc and minirouter, so the live guest
// image stands in for both.
func testDocsScripts(t *testing.T) {
	docs := os.Getenv("LIVE_DOCS_DIR")
	if docs == "" {
		t.Skip("LIVE_DOCS_DIR not set")
	}

	stageDocsImages(t)

	deadline := time.Now().Add(docsBudget)

	for _, s := range docsScripts {
		name := strings.TrimSuffix(filepath.Base(s.path), ".mm")

		t.Run(name, func(t *testing.T) {
			if time.Now().After(deadline) {
				t.Fatalf("the scripts before this one used the %v budget", docsBudget)
			}

			// minimega reads the script in the container, which mounts the
			// files directory at the same path
			data, err := os.ReadFile(filepath.Join(docs, s.path))
			if err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(harness.MMFiles, "docs-"+name+".mm")
			if err := os.WriteFile(script, data, 0644); err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() {
				os.Remove(script)

				for _, cmd := range s.after {
					harness.Raw(cmd)
				}
				for _, ns := range s.clear {
					// a script can end before it makes the namespace
					harness.Raw("clear namespace " + ns)
				}
			})

			cmd := "read " + script
			if s.namespace != "" {
				cmd = "namespace " + s.namespace + " " + cmd
			}

			start := time.Now()
			out, err := harness.Raw(cmd)
			t.Logf("%v ran for %v", s.path, time.Since(start).Round(time.Second))

			// keep what the script printed
			text := docsOutput(out)
			printed, _ := harness.Artifact(t, "docs-"+name+".txt")
			if werr := os.WriteFile(printed, []byte(text), 0644); werr == nil {
				harness.Keep(t, printed)
			}

			for _, want := range s.want {
				if !strings.Contains(text, want) {
					t.Errorf("the output of %v does not contain %q", s.path, want)
				}
			}
			if got := strings.Count(text, docsPingOK); got < s.pings {
				t.Errorf("the output of %v shows %d pings with all replies, want %d", s.path, got, s.pings)
			}

			if s.check != nil && err == nil {
				s.check(t)
			}

			if err != nil {
				t.Errorf("%v", err)

				for _, ns := range s.clear {
					vms, _ := harness.Raw(".columns name,state,cc_active,ip,vlan namespace " + ns + " vm info")
					t.Logf("VMs in namespace %v:\n%v", ns, docsOutput(vms))
				}
			}
		})
	}
}

// docsOutput renders the response of each command of a script. The responses
// of different commands have different columns, so they do not render as one
// table.
func docsOutput(resps minicli.Responses) string {
	var b strings.Builder

	for _, r := range resps {
		switch {
		case r.Error != "":
			b.WriteString("error: " + r.Error + "\n\n")
		case len(r.Header) > 0:
			b.WriteString(strings.Join(r.Header, " | ") + "\n")
			for _, row := range r.Tabular {
				b.WriteString(strings.Join(row, " | ") + "\n")
			}
			b.WriteString("\n")
		case r.Response != "":
			b.WriteString(strings.TrimRight(r.Response, "\n") + "\n\n")
		}
	}

	return b.String()
}

// stageDocsImages puts the live guest image in the files directory under the
// image names of the course. The minirouter initrd carries the router role in
// a file, because the scripts of the course set no kernel arguments. It also
// makes the ISO that chapter 14 expects.
func stageDocsImages(t *testing.T) {
	t.Helper()

	live := filepath.Join(harness.MMFiles, "live")
	kernel := filepath.Join(live, "live.kernel")
	initrd := filepath.Join(live, "live.initrd")

	link := func(src, name string) {
		dst := filepath.Join(harness.MMFiles, name)
		os.Remove(dst)
		if err := os.Link(src, dst); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(dst) })
	}

	link(kernel, "miniccc.kernel")
	link(initrd, "miniccc.initrd")
	link(kernel, "minirouter.kernel")

	router := filepath.Join(harness.MMFiles, "minirouter.initrd")
	if err := harness.InitrdWithFile(initrd, router, "etc/live.role", []byte("router\n")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(router) })

	iso := filepath.Join(harness.MMFiles, "lab.iso")
	if err := os.WriteFile(iso, make([]byte, 1<<20), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(iso) })
}
