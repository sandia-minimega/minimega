// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minicli

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// sortData is in a random order and the third column is the correct order.
var sortData = [][]string{
	[]string{"b", "b", "2"},
	[]string{"a", "a", "0"},
	[]string{"d", "a", "4"},
	[]string{"a", "b", "1"},
	[]string{"e", "a", "5"},
	[]string{"c", "a", "3"},
}

// sortDataInts is in a random order and the third column is the correct order.
var sortDataInts = [][]string{
	[]string{"2", "b", "2"},
	[]string{"0", "a", "0"},
	[]string{"4", "a", "4"},
	[]string{"1", "b", "1"},
	[]string{"5", "a", "5"},
	[]string{"3", "a", "3"},
}

func TestSort(t *testing.T) {
	data := make([][]string, len(sortData))
	copy(data, sortData)

	sort.Sort(table(data))

	for i := 0; i < len(data); i++ {
		v, _ := strconv.Atoi(data[i][2])
		if i != v {
			t.Errorf("out of order, %v: %v", i, data)
		}
	}
}

func TestSortInts(t *testing.T) {
	data := make([][]string, len(sortDataInts))
	copy(data, sortData)

	sort.Sort(table(data))

	for i := 0; i < len(data); i++ {
		v, _ := strconv.Atoi(data[i][2])
		if i != v {
			t.Errorf("out of order, %v: %v", i, data)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"abcdef", 0, "abcdef"},
		{"abcdef", 6, "abcdef"},
		{"abcdef", 10, "abcdef"},
		{"abcdef", 5, "ab..."},
		{"abcdef", 4, "a..."},
		{"abcdef", 3, "abc"},
		{"abcdef", 1, "a"},
		{"ééééé", 4, "é..."},
	}

	for _, c := range cases {
		if got := truncate(c.in, c.width); got != c.want {
			t.Errorf("truncate(%q, %v) = %q, want %q", c.in, c.width, got, c.want)
		}
	}
}

func widthResponses(width int, mode int) Responses {
	return Responses{&Response{
		Host:    "foo",
		Header:  []string{"name", "ip"},
		Tabular: [][]string{{"a", "[10.0.0.1 10.0.0.2 10.0.0.3]"}},
		Flags: &Flags{
			Headers: true,
			Width:   width,
			Mode:    mode,
		},
	}}
}

func TestStringWidth(t *testing.T) {
	want := "name | ip\na    | [10.0.0.1..."
	if got := widthResponses(12, defaultMode).String(); got != want {
		t.Errorf("got:\n%v\nwant:\n%v", got, want)
	}

	want = "name | ip\na    | [10.0.0.1 10.0.0.2 10.0.0.3]"
	if got := widthResponses(0, defaultMode).String(); got != want {
		t.Errorf("got:\n%v\nwant:\n%v", got, want)
	}
}

func TestFullString(t *testing.T) {
	r := widthResponses(12, defaultMode)
	orig := r[0].Flags
	flags := *orig

	want := "name | ip\na    | [10.0.0.1 10.0.0.2 10.0.0.3]"
	if got := r.FullString(); got != want {
		t.Errorf("got:\n%v\nwant:\n%v", got, want)
	}

	if r[0].Flags != orig || *orig != flags {
		t.Error("FullString modified flags")
	}
}

func TestRenderWithoutFlags(t *testing.T) {
	r := widthResponses(0, defaultMode)
	r[0].Flags = nil
	first := r[0]
	value := strings.Repeat("x", DefaultWidth+1)
	r[0].Tabular[0][1] = value

	want := "host | name | ip\nfoo  | a    | " + value
	if got := r.FullString(); got != want {
		t.Errorf("FullString = %q, want %q", got, want)
	}
	want = "host | name | ip\nfoo  | a    | " + strings.Repeat("x", DefaultWidth-3) + "..."
	if got := r.String(); got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
	if r[0] != first || r[0].Flags != nil {
		t.Error("rendering modified response")
	}
}

func TestRenderModes(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode int
		want string
	}{
		{"csv", csvMode, "name,ip\na,[10.0.0.1 10.0.0.2 10.0.0.3]"},
		{"json", jsonMode, `[{"Host":"foo","Response":"","Header":["name","ip"],"Tabular":[["a","[10.0.0.1 10.0.0.2 10.0.0.3]"]],"Error":"","Data":null}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := widthResponses(12, tc.mode)
			if got := r.String(); got != tc.want {
				t.Errorf("String = %q, want %q", got, tc.want)
			}
			if got := r.FullString(); got != tc.want {
				t.Errorf("FullString = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEmptyResponses(t *testing.T) {
	var r Responses
	if r.String() != "" || r.FullString() != "" {
		t.Error("expected empty output for empty responses")
	}
}

func TestRenderConcurrent(t *testing.T) {
	for _, defaults := range []bool{false, true} {
		r := widthResponses(12, defaultMode)
		if defaults {
			r[0].Flags = nil
		}
		// Exercise shared responses under the race detector without priming flags.
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = r.String()
				_ = r.FullString()
			}()
		}
		wg.Wait()
	}
}

func TestWidthBuiltin(t *testing.T) {
	Reset()

	orig := defaultFlags
	defer func() { defaultFlags = orig }()

	MustRegister(&Handler{
		Patterns: []string{".width [width]", ".width <width> (command)"},
		Call:     cliWidth,
	})
	MustRegister(&Handler{
		Patterns: []string{"tab"},
		Call: func(c *Command, out chan<- Responses) {
			out <- Responses{&Response{
				Host:    "h",
				Header:  []string{"col"},
				Tabular: [][]string{{"abcdefghij"}},
			}}
		},
	})

	run := func(cmd string) Responses {
		t.Helper()

		out, err := ProcessString(cmd, false)
		if err != nil {
			t.Fatalf("%v: %v", cmd, err)
		}

		var res Responses
		for r := range out {
			res = append(res, r...)
		}
		return res
	}

	if got := run(".width")[0].Response; got != strconv.Itoa(DefaultWidth) {
		t.Errorf("default width = %v, want %v", got, DefaultWidth)
	}

	if got := run(".width 5 tab").String(); got != "host | col\nh    | ab..." {
		t.Errorf("per-command width: got %q", got)
	}

	if defaultFlags.Width != DefaultWidth {
		t.Errorf("per-command width changed default: %v", defaultFlags.Width)
	}

	run(".width 0")
	if defaultFlags.Width != 0 {
		t.Errorf("global width = %v, want 0", defaultFlags.Width)
	}

	if r := run(".width -1"); r[0].Error == "" {
		t.Errorf("expected error for negative width")
	}

	if r := run(".width foo tab"); r[0].Error == "" {
		t.Errorf("expected error for invalid width")
	}
}
