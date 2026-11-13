// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

// Package harness is the shared support code for the live integration tests
// in the sibling group directories (core, ...): the command socket client,
// polling and cc helpers, and parsers for the artifacts minimega writes. It
// has no build tag, so its own unit tests run without a deployment. See
// tests/live/README.md.
package harness

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
)

// Environment knobs. Defaults match tests/live/run.sh and the Docker
// deployment described in docker/README.md.
var (
	// minimega -base directory holding the command socket (bind-mounted
	// from the container).
	mmBase = envOr("LIVE_MM_BASE", "/tmp/minimega")
	// minimega -filepath directory (bind-mounted from the container).
	MMFiles = envOr("LIVE_MM_FILES", filepath.Join(mmBase, "files"))
	// namespace created by topology.mm
	Namespace = envOr("LIVE_NAMESPACE", "live")
	// Docker container running minimega; used for host-side assertions
	// (Open vSwitch, host taps, processes). Empty disables those checks.
	container = envOr("LIVE_CONTAINER", "minimega")
	// URL where miniweb is reachable from the test host.
	MiniwebURL = envOr("LIVE_MINIWEB_URL", "http://127.0.0.1:9001")
	// directory for artifacts worth keeping (screenshots, pcaps)
	artifactDir = envOr("LIVE_ARTIFACTS", filepath.Join(os.TempDir(), "minimega-live-artifacts"))
	// how long to wait for the topology to settle (boot, DHCP, OSPF)
	SettleTimeout = envDuration("LIVE_SETTLE_TIMEOUT", 6*time.Minute)
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// mm is a thin wrapper around miniclient that runs commands in the test
// namespace and turns responses into errors and tables.
type mm struct {
	conn *miniclient.Conn
}

var client *mm

// Connect dials the minimega command socket. Call it once, from TestMain,
// before any other function in this package.
func Connect() error {
	c, err := dial()
	client = c
	return err
}

// Run runs a command in the test namespace.
func Run(cmd string) (minicli.Responses, error) { return client.run(cmd) }

// Text runs a command in the test namespace and returns the concatenated
// simple responses.
func Text(cmd string) (string, error) { return client.text(cmd) }

// Raw runs a command exactly as given, outside the test namespace.
func Raw(cmd string) (minicli.Responses, error) { return client.raw(cmd) }

func dial() (*mm, error) {
	conn, err := miniclient.Dial(mmBase)
	if err != nil {
		return nil, fmt.Errorf("dial minimega at %v: %w", mmBase, err)
	}
	return &mm{conn: conn}, nil
}

// raw runs a command exactly as given and returns every response from every
// host. Command errors are joined into the returned error.
func (m *mm) raw(cmd string) (minicli.Responses, error) {
	var out minicli.Responses
	var errs []string

	for r := range m.conn.Run(cmd) {
		if r.Status {
			continue
		}
		for _, resp := range r.Resp {
			if resp.Error != "" {
				errs = append(errs, fmt.Sprintf("%v: %v", resp.Host, resp.Error))
			}
			out = append(out, resp)
		}
	}

	if err := m.conn.Error(); err != nil {
		// the socket broke; reconnect so later subtests still get a chance
		if c, derr := dial(); derr == nil {
			m.conn = c.conn
		}
		return out, fmt.Errorf("%q: connection error: %w", cmd, err)
	}

	if len(errs) > 0 {
		return out, fmt.Errorf("%q: %v", cmd, strings.Join(errs, "; "))
	}

	return out, nil
}

// run runs a command in the test namespace.
func (m *mm) run(cmd string) (minicli.Responses, error) {
	return m.raw(fmt.Sprintf("namespace %v %v", Namespace, cmd))
}

// text runs a command and returns the concatenated simple responses.
func (m *mm) text(cmd string) (string, error) {
	resps, err := m.run(cmd)
	var b strings.Builder
	for _, r := range resps {
		b.WriteString(r.Response)
	}
	return b.String(), err
}

// Row is one tabular row keyed by column header. The responding host is
// stored under "host".
type Row map[string]string

// table runs a command and flattens tabular responses from all hosts.
func (m *mm) table(cmd string) ([]Row, error) {
	resps, err := m.run(cmd)
	return ToRows(resps), err
}

func ToRows(resps minicli.Responses) []Row {
	var rows []Row
	for _, r := range resps {
		for _, t := range r.Tabular {
			rw := Row{"host": r.Host}
			for i, h := range r.Header {
				if i < len(t) {
					rw[h] = t[i]
				}
			}
			rows = append(rows, rw)
		}
	}
	return rows
}

// must* helpers fail the current test on error.

func MustRun(t testing.TB, cmd string) minicli.Responses {
	t.Helper()
	resps, err := client.run(cmd)
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	return resps
}

func MustText(t testing.TB, cmd string) string {
	t.Helper()
	s, err := client.text(cmd)
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	return s
}

func MustTable(t testing.TB, cmd string) []Row {
	t.Helper()
	rows, err := client.table(cmd)
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	return rows
}

// VMInfo returns "vm info" rows keyed by VM name.
func VMInfo(t testing.TB) map[string]Row {
	t.Helper()
	vms := map[string]Row{}
	for _, r := range MustTable(t, "vm info") {
		vms[r["name"]] = r
	}
	return vms
}

// Eventually retries fn until it succeeds or the timeout expires.
func Eventually(t testing.TB, timeout, interval time.Duration, fn func() error) {
	t.Helper()
	if err := poll(timeout, interval, fn); err != nil {
		t.Fatalf("condition not met after %v: %v", timeout, err)
	}
}

func poll(timeout, interval time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		err := fn()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(interval)
	}
}

// List parses minimega's "[a, b]" list formatting.
func List(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// --- command and control ---------------------------------------------------

var ccSeq atomic.Int64

// CCRun issues a cc command (e.g. `exec hostname`, `send foo`) filtered to the
// given cc filter and waits until `want` clients have responded. It returns
// per-VM results read from the miniccc_responses directory and the command ID.
func CCRun(t testing.TB, filter, command string, want int, timeout time.Duration) (map[string]CCResult, int) {
	t.Helper()
	res, id, err := ccTry(filter, command, want, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return res, id
}

// ccTry is CCRun without failing the test, so callers can retry.
func ccTry(filter, command string, want int, timeout time.Duration) (map[string]CCResult, int, error) {
	prefix := fmt.Sprintf("live%d", ccSeq.Add(1))
	if err := ccIssue(filter, command, prefix); err != nil {
		return nil, -1, err
	}
	id, err := ccWait(prefix, command, want, timeout)
	if err != nil {
		return nil, id, err
	}
	res, err := ccRead(id)
	return res, id, err
}

// ccIssue sends a cc command under the given filter and prefix.
func ccIssue(filter, command, prefix string) error {
	cmds := []string{"clear cc filter", "cc prefix " + prefix, "cc " + command}
	if filter != "" {
		cmds = append(cmds[:1], append([]string{"cc filter " + filter}, cmds[1:]...)...)
	}
	var err error
	for _, c := range cmds {
		if _, err = client.run(c); err != nil {
			break
		}
	}
	// always reset shared cc state before checking the error
	client.run("clear cc prefix")
	client.run("clear cc filter")
	if err != nil {
		return fmt.Errorf("cc %v: %w", command, err)
	}
	return nil
}

// ccWait waits until the cc command with the given prefix has want
// responses and returns its ID (-1 if it never appeared).
func ccWait(prefix, command string, want int, timeout time.Duration) (int, error) {
	id := -1
	err := poll(timeout, 2*time.Second, func() error {
		rows, err := client.table("cc commands")
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r["prefix"] != prefix {
				continue
			}
			id, _ = strconv.Atoi(r["id"])
			if n, _ := strconv.Atoi(r["responses"]); n < want {
				return fmt.Errorf("cc command %v (%v): %d/%d responses", id, command, n, want)
			}
			return nil
		}
		return fmt.Errorf("cc command with prefix %v not found", prefix)
	})
	if err != nil {
		return id, fmt.Errorf("after %v: %w", timeout, err)
	}
	return id, nil
}

// ccRead reads the responses to a cc command, keyed by VM name.
func ccRead(id int) (map[string]CCResult, error) {
	rows, err := client.table("vm info")
	if err != nil {
		return nil, err
	}
	byUUID := map[string]string{}
	for _, r := range rows {
		byUUID[r["uuid"]] = r["name"]
	}
	return readResponses(filepath.Join(MMFiles, Namespace, "miniccc_responses", strconv.Itoa(id)), byUUID)
}

// CCExec runs a shell command on exactly one VM and returns its result.
func CCExec(t testing.TB, vm, script string) CCResult {
	t.Helper()
	r, err := ccExecTry(vm, script, 90*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// ccExecTry is CCExec without failing the test, so callers can retry.
func ccExecTry(vm, script string, timeout time.Duration) (CCResult, error) {
	res, _, err := ccTry("name="+vm, fmt.Sprintf("exec sh -c %q", script), 1, timeout)
	if err != nil {
		return CCResult{}, err
	}
	return execResult(res, vm)
}

// CCOK runs a shell command on one VM and fails the test unless it exits 0.
func CCOK(t testing.TB, vm, script string) string {
	t.Helper()
	r := CCExec(t, vm, script)
	if r.ExitCode != 0 {
		t.Fatalf("%v: `%v` exited %d\nstdout: %s\nstderr: %s", vm, script, r.ExitCode, r.Stdout, r.Stderr)
	}
	return r.Stdout
}

// CCEventually retries a shell command on one VM until it exits 0. A missing
// response is retried like a failing command, within the same timeout.
func CCEventually(t testing.TB, vm, script string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var out string
	Eventually(t, timeout, 3*time.Second, func() error {
		r, err := ccExecTry(vm, script, max(time.Until(deadline), 5*time.Second))
		if err != nil {
			return err
		}
		if r.ExitCode != 0 {
			return fmt.Errorf("%v: `%v` exited %d: %s%s", vm, script, r.ExitCode, r.Stdout, r.Stderr)
		}
		out = r.Stdout
		return nil
	})
	return out
}

// --- host side ---------------------------------------------------------------

// InContainer runs a command inside the minimega container. Tests that need
// it are skipped when LIVE_CONTAINER is empty.
func InContainer(t testing.TB, args ...string) (string, error) {
	t.Helper()
	if container == "" {
		t.Skip("LIVE_CONTAINER not set")
	}
	var out bytes.Buffer
	cmd := exec.Command("docker", append([]string{"exec", container}, args...)...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// Artifact returns a path for a file worth keeping, creating its directory.
// The second return value is the same path as seen from inside minimega, for
// commands that write files (screenshots, captures).
func Artifact(t testing.TB, name string) (host, mmPath string) {
	t.Helper()
	dir := filepath.Join(MMFiles, Namespace, "artifacts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	host = filepath.Join(dir, name)
	// MMFiles is bind-mounted at the same path inside the container
	mmPath = host
	return host, mmPath
}

// Keep copies a file into the artifact directory uploaded by CI.
func Keep(t testing.TB, src string) {
	t.Helper()
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		t.Logf("artifact dir: %v", err)
		return
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Logf("keep %v: %v", src, err)
		return
	}
	dst := filepath.Join(artifactDir, filepath.Base(src))
	if err := os.WriteFile(dst, b, 0644); err != nil {
		t.Logf("keep %v: %v", src, err)
	}
}
