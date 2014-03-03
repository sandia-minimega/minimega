// Copyright 2014 David Fritz. All rights reserved.
// 
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
// 
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License for
// more details.
// 
// You should have received a copy of the GNU General Public License along with
// this program.  If not, see <http://www.gnu.org/licenses/>.

package gomacro_test

import (
	. "gomacro"
	"testing"
)

func TestDefine(t *testing.T) {
	m := NewMacro()
	err := m.Define("key", "value")
	if err != nil {
		t.Errorf(err.Error())
	}
	got := m.Macro("key")
	want := "value"
	if got != want {
		t.Errorf("define got \"%v\", wanted \"%v\"", got, want)
	}

	// also test getting something that shouldn't exist
	got = m.Macro("foo")
	want = ""
	if got != want {
		t.Errorf("Define got \"%v\", wanted \"%v\"", got, want)
	}
}

func TestUndefine(t *testing.T) {
	m := NewMacro()
	m.Define("key", "value")
	m.Undefine("key")
	got := m.Macro("key")
	want := ""
	if got != want {
		t.Errorf("Undefine got \"%v\", wanted \"%v\"", got, want)
	}
}

func TestList(t *testing.T) {
	m := NewMacro()
	m.Define("key", "value")
	m.Define("foo", "value")
	m.Define("bar", "value")
	got := m.List()
	want := []string{"key", "foo", "bar"}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("List got \"%v\", wanted \"%v\"", got, want)
		}
	}
}

func TestMacro(t *testing.T) {
	m := NewMacro()
	m.Define("key", "value")
	got := m.Macro("key")
	want := "value"
	if got != want {
		t.Errorf("Macro got \"%v\", wanted \"%v\"", got, want)
	}
}

func TestParseReplace(t *testing.T) {
	m := NewMacro()
	m.Define("key", "value")
	got := m.Parse("this is my key, my key is this")
	want := "this is my value, my value is this"
	if got != want {
		t.Errorf("ParseReplace got \"%v\", wanted \"%v\"", got, want)
	}
}

func TestParseReplaceRecursive(t *testing.T) {
	m := NewMacro()
	m.Define("key", "value")
	m.Define("value", "foo")
	got := m.Parse("this is my key, my value is this")
	want := "this is my foo, my foo is this"
	if got != want {
		t.Errorf("ParseReplace got \"%v\", wanted \"%v\"", got, want)
	}
}

func TestParseFunction(t *testing.T) {
	m := NewMacro()
	err := m.Define("key(x)", "value x")
	if err != nil {
		t.Errorf(err.Error())
	}
	got := m.Parse("this is key(my) key")
	want := "this is value my key"
	if got != want {
		t.Errorf("ParseFunction got \"%v\", wanted \"%v\"", got, want)
	}
}

func TestParseFunctionCompound(t *testing.T) {
	m := NewMacro()
	err := m.Define("key(x)", "value x")
	if err != nil {
		t.Errorf(err.Error())
	}
	err = m.Define("value", "foo")
	if err != nil {
		t.Errorf(err.Error())
	}
	got := m.Parse("this is key(my) key")
	want := "this is foo my key"
	if got != want {
		t.Errorf("ParseFunction got \"%v\", wanted \"%v\"", got, want)
	}
}
