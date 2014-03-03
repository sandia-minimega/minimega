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

// gomacro - a simple textual macro expansion library.  gomacro stores
// keys with text expansions, similar to the C preprocessor. It also
// supports function-like macros and concatenation.
//
// gomacro tokenizes on whitespace, and will recursively expand emitted
// text.
package gomacro

import (
	"fmt"
	"regexp"
	"strings"
)

type Macro struct {
	macros map[string]*macro
}

type macro struct {
	original  string
	expansion string
	args      []string
	re        *regexp.Regexp
}

// Return a new, empty macro parsing object.
func NewMacro() *Macro {
	return &Macro{
		macros: make(map[string]*macro),
	}
}

// Add a new, or overwrites an existing macro definition.
func (m *Macro) Define(key, expansion string) error {
	// keys can be in the form of:
	//	literal : ^[:alnum:]+$
	//	function: ^[:alnum:]\\([:alnum:]+(,[:alnum:])*\\)$
	var err error
	var k string
	ret := &macro{
		expansion: expansion,
	}
	matchLiteral, err := regexp.MatchString("^[a-zA-Z0-9]+$", key)
	if err != nil {
		return err
	}
	matchFunction, err := regexp.MatchString("^[a-zA-Z0-9]+\\([a-zA-Z0-9]+(,[a-zA-Z0-9]+)*\\)$", key)
	if err != nil {
		return err
	}

	if matchLiteral {
		k = key
		ret.original = key
		ret.re, err = regexp.Compile(key)
		if err != nil {
			return err
		}
	} else if matchFunction {
		f := strings.Split(key, "(")
		if len(f) != 2 {
			return fmt.Errorf("malformed key %v", key)
		}
		f[1] = strings.Trim(f[1], ")")
		ret.args = strings.Split(f[1], ",")
		k = f[0]
		ret.original = key
		r := k + "\\("
		for i, _ := range ret.args {
			if i != 0 {
				r += ","
			}
			r += "[a-zA-Z0-9]+"
		}
		r += "\\)"
		ret.re, err = regexp.Compile(r)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("invalid macro: %v", key)
	}
	m.macros[k] = ret
	return nil
}

// Remove an existing macro definition.
func (m *Macro) Undefine(key string) {
	if _, ok := m.macros[key]; ok {
		delete(m.macros, key)
	}
}

// Return a list of macros currently set.
func (m *Macro) List() []string {
	var keys []string
	for k, _ := range m.macros {
		keys = append(keys, k)
	}
	return keys
}

// Return the macro text for a given key.
func (m *Macro) Macro(key string) (string, string) {
	if v, ok := m.macros[key]; ok {
		return v.original, v.expansion
	}
	return "", ""
}

// Parse input text with set macros.
func (m *Macro) Parse(input string) string {
	for _, v := range m.macros {
		output := v.re.ReplaceAllStringFunc(input, v.expand)
		if input != output {
			return m.Parse(output)
		}
	}
	return input
}

func (m *macro) expand(input string) string {
	if len(m.args) == 0 {
		return m.expansion
	}
	// create a new macro with the parametric args and parse it
	f := strings.Split(input, "(")
	if len(f) != 2 {
		return ""
	}
	f[1] = strings.Trim(f[1], ")")
	args := strings.Split(f[1], ",")
	nm := NewMacro()
	for i, v := range m.args {
		err := nm.Define(v, args[i])
		if err != nil {
			return ""
		}
	}
	return nm.Parse(m.expansion)
}
