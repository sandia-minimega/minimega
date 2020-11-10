// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Modified from the original source code.
// Original source code can be found at https://github.com/golang/go/blob/master/doc/codewalk/markov.go

package main

import (
	"math/rand"
	"strings"
)

// Prefix is a Markov chain prefix of one or more words.
type Prefix []string

// String returns the Prefix as a string (for use as a map key).
func (p Prefix) String() string {
	return strings.Join(p, " ")
}

// Shift removes the first word from the Prefix and appends the given word.
func (p Prefix) Shift(word string) {
	copy(p, p[1:])
	p[len(p)-1] = word
}

// Chain contains a map ("chain") of prefixes to a list of suffixes.
// A prefix is a string of prefixLen words joined with spaces.
// A suffix is a single word. A prefix can have multiple suffixes.
type Chain struct {
	chain     map[string][]string
	prefixLen int
}

// Build builds the chain using the given []string parameter
func (c *Chain) Build(s []string) {
	for _, a := range s {
		p := make(Prefix, c.prefixLen)
		for _, b := range strings.Split(a, " ") {
			key := p.String()
			c.chain[key] = append(c.chain[key], b)
			p.Shift(b)
		}
	}
}

// Generate returns a string of at most n words generated from Chain.
func (c *Chain) Generate() string {
	p := make(Prefix, c.prefixLen)
	var words []string
	for {
		choices := c.chain[p.String()]
		if len(choices) == 0 {
			break
		}
		next := choices[rand.Intn(len(choices))]
		words = append(words, next)
		p.Shift(next)
	}
	return strings.Join(words, " ")
}

// NewChain returns a new Chain with prefixes of prefixLen words.
func NewChain() *Chain {
	return &Chain{make(map[string][]string), 2}
}
