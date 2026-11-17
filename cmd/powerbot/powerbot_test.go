package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConf(t *testing.T, s string) string {
	p := filepath.Join(t.TempDir(), "powerbot.conf")
	if err := os.WriteFile(p, []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadConfigShortLines(t *testing.T) {
	for _, line := range []string{
		"node n1",
		"node n1 pdu1",
		"node n1 pdu1 3 10.0.0.5",
		"node n1 pdu1 3 10.0.0.5 user",
		"ipmi",
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%q: readConfig panicked: %v", line, r)
				}
			}()
			if _, err := readConfig(writeConf(t, line+"\n")); err == nil {
				t.Errorf("%q: expected a config error, got nil", line)
			}
		}()
	}
}

func TestReadConfigValidNodes(t *testing.T) {
	c, err := readConfig(writeConf(t, "ipmi /usr/bin/ipmitool\nnode n1 pdu1 3\nnode n2 pdu1 4 10.0.0.5 u p\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.nodes) != 2 || c.ipmiPath != "/usr/bin/ipmitool" || c.ipmis["n2"].password != "p" {
		t.Errorf("unexpected config: %+v", c)
	}
}

func TestReadConfigBadLoglevel(t *testing.T) {
	c, err := readConfig(writeConf(t, "loglevel verbos\nnode n1 pdu1 3\n"))
	if err == nil && len(c.nodes) == 0 {
		t.Error("bad loglevel silently dropped the rest of the config with nil error")
	}
}

// A malformed node line must not echo its IPMI password into the error,
// which main passes to log.Fatal (stderr/syslog).
func TestReadConfigErrorHidesPassword(t *testing.T) {
	_, err := readConfig(writeConf(t, "node n1 pdu1 3 10.0.0.5 admin s3cret extra\n"))
	if err == nil {
		t.Fatal("expected a config error for an 8-field node line")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks the password: %v", err)
	}
}
