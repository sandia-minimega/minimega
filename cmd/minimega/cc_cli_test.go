// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"reflect"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

func TestCCCommandPreProcess(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *ron.Command
		wantErr bool
	}{
		{
			name: "no options",
			args: []string{"cat", "foo"},
			want: &ron.Command{Command: []string{"cat", "foo"}},
		},
		{
			name: "stdio",
			args: []string{"stdin=in", "stdout=out", "stderr=err", "cat"},
			want: &ron.Command{Stdin: "in", Stdout: "out", Stderr: "err", Command: []string{"cat"}},
		},
		{
			name: "session",
			args: []string{"session=user", "notepad.exe"},
			want: &ron.Command{Session: ron.SessionUser, Command: []string{"notepad.exe"}},
		},
		{
			name: "session with stdio",
			args: []string{"stdout=out", "session=elevated", "stdin=in", "stderr=err", "cat"},
			want: &ron.Command{
				Stdin: "in", Stdout: "out", Stderr: "err",
				Session: ron.SessionElevated,
				Command: []string{"cat"},
			},
		},
		{
			name:    "invalid session",
			args:    []string{"session=root", "cat"},
			wantErr: true,
		},
		{
			name:    "options without a command",
			args:    []string{"stdout=out", "session=user"},
			wantErr: true,
		},
		{
			name: "unknown key starts the command",
			args: []string{"FOO=bar", "session=user"},
			want: &ron.Command{Command: []string{"FOO=bar", "session=user"}},
		},
		{
			name: "options after the command are arguments",
			args: []string{"echo", "session=user"},
			want: &ron.Command{Command: []string{"echo", "session=user"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ccCommandPreProcess(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
