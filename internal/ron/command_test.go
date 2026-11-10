// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"reflect"
	"testing"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func TestCommandCopy(t *testing.T) {
	level := log.DEBUG

	c := &Command{
		ID:         1,
		Background: true,
		Command:    []string{"echo", "hello"},
		FilesSend:  []string{"send"},
		FilesRecv:  []string{"recv"},
		ConnTest:   &ConnTest{Endpoint: "tcp://10.0.0.1:80", Wait: time.Second, Packet: []byte{1}},
		PID:        2,
		KillAll:    "name",
		Level:      &level,
		Filter:     &Filter{UUID: "uuid", Tags: map[string]string{"k": "v"}},
		CheckedIn:  []string{"client"},
		Issued:     3,
		Prefix:     "prefix",
		Once:       true,
		Sent:       true,
		Stdin:      "in",
		Stdout:     "out",
		Stderr:     "err",
		Session:    SessionUser,
	}

	// every field must be set for the comparison below to cover it
	v := reflect.ValueOf(*c)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Fatalf("field %s is not set", v.Type().Field(i).Name)
		}
	}

	if got := c.Copy(); !reflect.DeepEqual(got, c) {
		t.Fatalf("got %+v, want %+v", got, c)
	}
}
