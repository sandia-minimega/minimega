// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ranges

import (
	"fmt"
	"sort"
	"strconv"
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

// Flatten returns a compact representation of the trie with numbers replaced
// with range notations
func (t *trieNode) Flatten() []string {
	var res []string

	if t.Terminal {
		res = append(res, "")
	}

	children := t.sortedChildren()

	// children that are numbers and their descendants that are non-numbers
	// keyed by the digit string so that "01" and "1" stay distinct
	nums := map[string]*trieNode{}

	for _, k := range children {
		// record numeric children for later
		if unicode.IsNumber(k) {
			for i, child := range t.Children[k].flattenNums(k) {
				nums[i] = child
			}

			continue
		}

		// append non-numeric children immediately
		for _, suffix := range t.Children[k].Flatten() {
			res = append(res, fmt.Sprintf("%c%s", k, suffix))
		}
	}

	// suffixes and the digit strings that they correspond to
	vals := map[string][]string{}

	for i, descendant := range nums {
		for _, suffix := range descendant.Flatten() {
			vals[suffix] = append(vals[suffix], i)
		}
	}

	// again, sort for determinism
	suffixes := []string{}

	for suffix := range vals {
		suffixes = append(suffixes, suffix)
	}

	sort.Strings(suffixes)

	// combine each group with the same suffix and padding
	for _, suffix := range suffixes {
		groups, literals := groupPadded(vals[suffix])

		for _, pad := range sortedInts(groups) {
			res = append(res, unsplitPadded(groups[pad], pad)+suffix)
		}

		sort.Strings(literals)
		for _, l := range literals {
			res = append(res, l+suffix)
		}
	}

	return res
}

// sortedChildren returns the children rune in sorted order
func (t *trieNode) sortedChildren() []rune {
	// runes of children to sort
	var res []rune

	// flatten all the children that are numbers first
	for k := range t.Children {
		res = append(res, k)
	}

	// sort child runes so that there's some determinism on ordering
	sort.Slice(res, func(i, j int) bool {
		return res[i] < res[j]
	})

	return res
}

// flattenNums converts trieNodes from a given point into ints and returns
// pointers to the children. If r is not a number, returns an empty map.
func (t *trieNode) flattenNums(r rune) map[string]*trieNode {
	res := map[string]*trieNode{}

	for k, child := range t.Children {
		if !unicode.IsNumber(k) {
			continue
		}

		for k, v := range child.flattenNums(k) {
			res[string(r)+k] = v
		}
	}

	if len(res) == 0 {
		res[string(r)] = t
		return res
	}

	// t is itself a number ("1" in "h1" and "h12"): keep it along with its
	// non-numeric suffixes, which the digit children above do not cover.
	if own := t.withoutNumbers(); own.Terminal || len(own.Children) > 0 {
		res[string(r)] = own
	}

	return res
}

// groupPadded groups digit strings by zero-padding width (0 for unpadded).
// A number with a leading zero fixes the width for its length; numbers of the
// same length without leading zeros join that group. Digit strings that do not
// fit in an int are returned as literals so they are never merged.
func groupPadded(digits []string) (map[int][]int, []string) {
	widths := map[int]bool{}
	for _, d := range digits {
		if len(d) > 1 && d[0] == '0' {
			widths[len(d)] = true
		}
	}

	groups := map[int][]int{}
	var literals []string

	for _, d := range digits {
		n, err := strconv.Atoi(d)
		if err != nil {
			literals = append(literals, d)
			continue
		}

		pad := 0
		if widths[len(d)] {
			pad = len(d)
		}
		groups[pad] = append(groups[pad], n)
	}

	return groups, literals
}

func sortedInts(m map[int][]int) []int {
	var res []int
	for k := range m {
		res = append(res, k)
	}
	sort.Ints(res)
	return res
}

// withoutNumbers returns a copy of t that only keeps non-numeric children.
func (t *trieNode) withoutNumbers() *trieNode {
	res := newTrie()
	res.Terminal = t.Terminal

	for k, child := range t.Children {
		if !unicode.IsNumber(k) {
			res.Children[k] = child
		}
	}

	return res
}
