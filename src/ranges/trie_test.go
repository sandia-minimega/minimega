// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package ranges

import "testing"

var testStrings = []string{
	"n0",
	"n1",
	"n2",
	"node0",
	"node1",
	"node2",
	"node3",
	"node4",
	"node5",
	"node50",
	"foo0",
	"foo1",
	"bar",
}

var testPrefixes = []string{
	"n", "node", "foo", "bar",
}

func TestTrieAdd(t *testing.T) {
	trie := newTrie()

	for _, v := range testStrings {
		trie.Add(v)
	}

	r := trie.Strings()

	// Make sure we can retrieve all the strings
	if len(testStrings) != len(r) {
		t.Errorf("got %d strings, expected %d", len(r), len(testStrings))
	}
}

func TestTrieDedup(t *testing.T) {
	trie := newTrie()

	for i := 0; i < 10; i++ {
		trie.Add("foobar")
	}

	r := trie.Strings()

	if len(r) != 1 {
		t.Errorf("got %d strings, expected 1", len(r))
	} else if r[0] != "foobar" {
		t.Errorf("got '%s', expected 'foobar'", r[0])
	}
}

func TestTriePrefixes(t *testing.T) {
	trie := newTrie()

	for _, v := range testStrings {
		trie.Add(v)
	}

	r := trie.AlphaPrefixes()
	if len(testPrefixes) != len(r) {
		t.Errorf("got %d strings, expected %d", len(r), len(testPrefixes))
	}
}
