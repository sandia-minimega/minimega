package minicli

import (
	"fmt"
)

// Validate checks for ambiguous patterns
func Validate() error {
	patterns := map[string][]PatternItem{}

	// Collect all the patterns across all the handlers
	for _, h := range handlers {
		for i, pattern := range h.Patterns {
			if _, ok := patterns[pattern]; ok {
				return fmt.Errorf("duplicate pattern: `%v`", pattern)
			}

			patterns[pattern] = h.PatternItems[i]
		}
	}

	// Create slice from pattern strings so the order is fixed
	slice := []string{}
	for pattern := range patterns {
		slice = append(slice, pattern)
	}

	// Test each pattern for ambiguity against all other patterns
	for i, pattern := range slice {
		for _, other := range slice[i+1:] {
			if ambiguous(patterns[pattern], patterns[other]) {
				return fmt.Errorf("ambiguous patterns: `%v` and `%v`", pattern, other)
			}
		}
	}

	return nil
}

func ambiguous(p0, p1 []PatternItem) bool {
	if len(p0) == 0 && len(p1) == 0 {
		return true
	} else if len(p0) == 0 && len(p1) > 0 {
		// If the next element of p1 is optional, patterns are ambiguous since
		// optional arguments have to come last.
		return p1[0].IsOptional()
	} else if len(p0) > 0 && len(p1) == 0 {
		// Same case as above.
		return p0[0].IsOptional()
	}

	// At least one item in each pattern
	item0, item1 := p0[0], p1[0]

	// Both optional
	if item0.IsOptional() && item1.IsOptional() {
		return true
	}

	// A list can always match anything in the other pattern
	if item0.IsList() || item1.IsList() {
		return true
	}

	allowed0, allowed1 := allowedValues(item0), allowedValues(item1)

	var match bool
	for _, val0 := range allowed0 {
		for _, val1 := range allowed1 {
			match = match || val0 == val1 || val0 == "*" || val1 == "*"
		}
	}

	if !match {
		return false
	}

	return ambiguous(p0[1:], p1[1:])
}

func allowedValues(item PatternItem) []string {
	vals := []string{}

	switch item.Type {
	case literalItem:
		vals = append(vals, item.Text)
	case choiceItem, choiceItem | optionalItem:
		for _, choice := range item.Options {
			vals = append(vals, choice)
		}
	case stringItem, stringItem | optionalItem, listItem, listItem | optionalItem:
		vals = append(vals, "*")
	case commandItem:
		// TODO: This is overly restrictive... we could be more precise by
		// going through all the handlers and pulling out the first element
		// from all their patterns.
		vals = append(vals, "*")
	}

	if item.IsOptional() {
		vals = append(vals, "")
	}

	return vals
}
