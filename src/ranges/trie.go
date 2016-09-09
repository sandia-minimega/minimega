// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ranges

import (
	"fmt"
	"unicode"
)

type trieNode struct {
	Children map[rune]*trieNode
	Terminal bool
}

func newTrie() *trieNode {
	return &trieNode{
		Children: make(map[rune]*trieNode),
	}
}

// Add a string to the trie
func (t *trieNode) Add(s string) {
	t.add([]rune(s))
}

// Add a slice of runes to the trie
func (t *trieNode) add(s []rune) {
	if len(s) == 0 {
		t.Terminal = true
		return
	}

	if _, ok := t.Children[s[0]]; !ok {
		t.Children[s[0]] = newTrie()
	}

	t.Children[s[0]].add(s[1:])
}

// Strings returns the unique strings stored in the trie
func (t *trieNode) Strings() []string {
	res := []string{}

	if t.Terminal {
		res = append(res, "")
	}

	for k, child := range t.Children {
		for _, suffix := range child.Strings() {
			res = append(res, fmt.Sprintf("%c%s", k, suffix))
		}
	}

	return res
}

// AlphaPrefixes find alphabetical prefixes among the unique strings stored in
// the trie.
func (t *trieNode) AlphaPrefixes() []string {
	var numericChild bool

	res := []string{}

	for k, child := range t.Children {
		if unicode.IsLetter(k) {
			for _, suffix := range child.AlphaPrefixes() {
				res = append(res, fmt.Sprintf("%c%s", k, suffix))
			}
		} else {
			numericChild = true
		}
	}

	if t.Terminal || numericChild {
		res = append(res, "")
	}

	return res
}
