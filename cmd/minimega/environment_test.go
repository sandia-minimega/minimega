// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetEnvironmentFlagDefaults(t *testing.T) {
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	base := fs.String("base", BASE_PATH, "")
	port := fs.Int("port", 9000, "")

	env := map[string]string{
		"MM_BASE": "/var/run/minimega",
		"MM_PORT": "9100",
	}
	lookupEnv := func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
	mappings := []environmentFlag{
		{"MM_BASE", "base"},
		{"MM_PORT", "port"},
	}

	if err := setEnvironmentFlagDefaults(fs, mappings, lookupEnv); err != nil {
		t.Fatal(err)
	}

	if got, want := *base, "/var/run/minimega"; got != want {
		t.Errorf("base: got %q, want %q", got, want)
	}
	if got, want := *port, 9100; got != want {
		t.Errorf("port: got %d, want %d", got, want)
	}
	if got, want := fs.Lookup("base").DefValue, "/var/run/minimega"; got != want {
		t.Errorf("base default: got %q, want %q", got, want)
	}

	var visited []string
	fs.Visit(func(f *flag.Flag) {
		visited = append(visited, f.Name)
	})
	if len(visited) != 0 {
		t.Errorf("environment defaults marked flags as command-line arguments: %v", visited)
	}

	if err := fs.Parse([]string{"-base", "/cli/minimega"}); err != nil {
		t.Fatal(err)
	}
	if got, want := *base, "/cli/minimega"; got != want {
		t.Errorf("command-line base: got %q, want %q", got, want)
	}
	if got, want := *port, 9100; got != want {
		t.Errorf("environment port after parsing: got %d, want %d", got, want)
	}
}

func TestSetEnvironmentFlagDefaultsInvalidValue(t *testing.T) {
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	fs.Int("port", 9000, "")

	err := setEnvironmentFlagDefaults(
		fs,
		[]environmentFlag{{"MM_PORT", "port"}},
		func(string) (string, bool) { return "not-a-port", true },
	)
	if err == nil {
		t.Fatal("expected invalid environment value to fail")
	}
	if got := err.Error(); !strings.Contains(got, "MM_PORT") || !strings.Contains(got, "not-a-port") {
		t.Errorf("error %q does not identify the environment variable and value", got)
	}
}

func TestLocalClientRequested(t *testing.T) {
	tests := []struct {
		name    string
		attach  bool
		execute bool
		pipe    string
		want    bool
	}{
		{name: "server", want: false},
		{name: "attach", attach: true, want: true},
		{name: "execute", execute: true, want: true},
		{name: "pipe", pipe: "minimega//test", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := localClientRequested(test.attach, test.execute, test.pipe); got != test.want {
				t.Errorf("localClientRequested() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestApplyEnvironmentFlagDefaultsFromConfigForPipe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minimega.conf")
	config := `
MM_BASE='/from config'
MM_PORT=9100
`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	base := fs.String("base", BASE_PATH, "")
	registerEnvironmentFlags(fs)

	readConfig := localClientRequested(false, false, "minimega//test")
	if err := applyEnvironmentFlagDefaultsFrom(fs, path, readConfig, func(string) (string, bool) {
		return "", false
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := *base, "/from config"; got != want {
		t.Errorf("base: got %q, want %q", got, want)
	}
	if got, want := fs.Lookup("port").DefValue, "9000"; got != want {
		t.Errorf("port default: got %q, want %q; config file should only supply the client base", got, want)
	}
}

func TestApplyEnvironmentFlagDefaultsCommandLineSkipsConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minimega.conf")
	if err := os.WriteFile(path, []byte("MM_BASE=\xff\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	base := fs.String("base", BASE_PATH, "")
	registerEnvironmentFlags(fs)

	if err := fs.Parse([]string{"-base", "/from-command-line"}); err != nil {
		t.Fatal(err)
	}
	if err := applyEnvironmentFlagDefaultsFrom(fs, path, true, func(name string) (string, bool) {
		return "", false
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := *base, "/from-command-line"; got != want {
		t.Errorf("base: got %q, want %q", got, want)
	}
}

func TestApplyEnvironmentFlagDefaultsEnvironmentSkipsConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minimega.conf")
	if err := os.WriteFile(path, []byte("MM_BASE=\xff\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	base := fs.String("base", BASE_PATH, "")
	registerEnvironmentFlags(fs)

	if err := applyEnvironmentFlagDefaultsFrom(fs, path, true, func(name string) (string, bool) {
		if name == "MM_BASE" {
			return "/from-environment", true
		}
		return "", false
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := *base, "/from-environment"; got != want {
		t.Errorf("base: got %q, want %q", got, want)
	}
}

func TestParseBaseEnvironmentFileInvalidUTF8(t *testing.T) {
	_, _, err := parseBaseEnvironmentFile([]byte("MM_BASE=\xff\n"))
	if err == nil {
		t.Fatal("expected invalid UTF-8 to fail")
	}
}

func TestParseBaseEnvironmentFileSystemdSyntax(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "unquoted escape",
			input: "MM_BASE=/tmp/minimega\\ data   \n",
			want:  "/tmp/minimega data",
		},
		{
			name:  "unquoted continuation",
			input: "MM_BASE=/tmp/minimega\\\n-data\n",
			want:  "/tmp/minimega-data",
		},
		{
			name:  "single quoted multiline",
			input: "MM_BASE=' /tmp/minimega\n data '\n",
			want:  " /tmp/minimega\n data ",
		},
		{
			name:  "double quoted escapes",
			input: "MM_BASE=\"/tmp/\\$minimega\\q\"\n",
			want:  "/tmp/$minimega\\q",
		},
		{
			name:  "quoted fragments",
			input: "MM_BASE=\"/tmp/\" 'minimega' data\n",
			want:  "/tmp/minimegadata",
		},
		{
			name:  "comment markers in value",
			input: "; ignored\nMM_BASE=/tmp/minimega #active\n",
			want:  "/tmp/minimega #active",
		},
		{
			name:  "last assignment wins",
			input: "MM_BASE=/old\nMM_BASE=/new\n",
			want:  "/new",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base, found, err := parseBaseEnvironmentFile([]byte(test.input))
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("MM_BASE not found")
			}
			if got := base; got != test.want {
				t.Errorf("MM_BASE: got %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseBaseEnvironmentFileMissing(t *testing.T) {
	base, found, err := parseBaseEnvironmentFile([]byte("MM_PORT=9000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("MM_BASE unexpectedly found with value %q", base)
	}
}

func TestApplyEnvironmentFlagDefaultsMissingConfig(t *testing.T) {
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	fs.String("base", BASE_PATH, "")
	registerEnvironmentFlags(fs)

	err := applyEnvironmentFlagDefaultsFrom(
		fs,
		filepath.Join(t.TempDir(), "missing"),
		true,
		func(string) (string, bool) { return "", false },
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplyEnvironmentFlagDefaultsSkipsConfigForServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minimega.conf")
	if err := os.WriteFile(path, []byte("MM_BASE=/from-config\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	base := fs.String("base", BASE_PATH, "")
	registerEnvironmentFlags(fs)

	if err := applyEnvironmentFlagDefaultsFrom(fs, path, false, func(string) (string, bool) {
		return "", false
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := *base, BASE_PATH; got != want {
		t.Errorf("base: got %q, want %q", got, want)
	}
}

func TestEnvironmentFlagsRegistered(t *testing.T) {
	for _, mapping := range environmentFlags {
		if flag.Lookup(mapping.flag) == nil {
			t.Errorf("%s references unknown flag -%s", mapping.environment, mapping.flag)
		}
	}
}

func registerEnvironmentFlags(fs *flag.FlagSet) {
	fs.Uint("degree", 0, "")
	fs.Uint("msa", 10, "")
	fs.String("broadcast", "", "")
	fs.String("vlanrange", "", "")
	fs.Int("port", 9000, "")
	fs.Bool("force", false, "")
	fs.Bool("recover", false, "")
	fs.Bool("nostdin", false, "")
	fs.String("context", "", "")
	fs.String("filepath", "", "")
	fs.String("level", "", "")
	fs.String("logfile", "", "")
	fs.String("cgroup", "", "")
	fs.Bool("panic", false, "")
	fs.Bool("abssnapshot", false, "")
}
