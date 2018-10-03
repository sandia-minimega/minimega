// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	"fmt"
	"strings"

	log "minilog"
)

type patternTrieKey struct {
	Type  itemType
	Value string
}

// patternTrie indexes the patterns from all the handlers to make command
// compilation faster.
type patternTrie struct {
	Children map[patternTrieKey]*patternTrie
	Handler  *Handler
}

// Add a new handler to the patternTrie. Returns an error if there is a
// conflict.
func (p *patternTrie) Add(h *Handler) error {
	for _, v := range h.PatternItems {
		if err := p.add(v, h); err != nil {
			return err
		}
	}

	return nil
}

// Add a pattern to the patternTrie. Returns an error if there is a conflict.
func (p *patternTrie) add(v []PatternItem, h *Handler) error {
	// ran out of items
	if len(v) == 0 {
		return p.setHandler(h)
	}

	// when we search for handlers, we don't want to have to deal with optional
	// things so set the handler at this level and the next level
	if v[0].IsOptional() {
		if err := p.setHandler(h); err != nil {
			return err
		}
	}

	var keys []patternTrieKey

	switch {
	case v[0].IsLiteral():
		keys = append(keys, patternTrieKey{
			Type:  v[0].Type,
			Value: v[0].Text,
		})
	case v[0].IsChoice():
		// explode all the options into literals
		for _, opt := range v[0].Options {
			keys = append(keys, patternTrieKey{
				Type:  literalItem,
				Value: opt,
			})
		}
	default:
		keys = append(keys, patternTrieKey{
			Type:  v[0].Type,
			Value: v[0].Key,
		})
	}

	for _, key := range keys {
		// clear optional since we have already dealt with it
		key.Type = key.Type & ^optionalItem

		if _, ok := p.Children[key]; !ok {
			p.Children[key] = &patternTrie{
				Children: make(map[patternTrieKey]*patternTrie),
			}
		}

		if err := p.Children[key].add(v[1:], h); err != nil {
			return err
		}
	}

	return nil
}

func (p *patternTrie) setHandler(h *Handler) error {
	if p.Handler != nil {
		return fmt.Errorf("conflicting handlers: %v and %v", p.Handler, h)
	}

	p.Handler = h
	return nil
}

// findHandler finds the handler based on the input.
//
// TODO: we should change this so that it actually does the compilation and
// returns a command.
func (p *patternTrie) findHandler(input inputItems) *Handler {
	if len(input) == 0 {
		return p.Handler
	}

	var handlers []*Handler

	for k, v := range p.Children {
		switch k.Type {
		case literalItem:
			// failed to match literal
			if !strings.HasPrefix(k.Value, input[0].Value) {
				continue
			}
		case stringItem:
			// anything matches
		case listItem:
			// anything matches and throw away rest of input
		case commandItem:
			// can't make a wrong choice (well, not at this stage at least)
			handlers = append(handlers, p.Handler)
			continue
		case choiceItem:
			// this should never happen since we expand all choices into
			// literals in patternTrie.add
		}

		// must be a candidate
		if h := v.findHandler(input[1:]); h != nil {
			handlers = append(handlers, h)
		}
	}

	switch len(handlers) {
	case 0:
		return nil
	case 1:
		return handlers[0]
	default:
		// wtf
		log.Warn("multiple handlers matched!?")
		return nil
	}
}

func (p *patternTrie) dump(depth int) {
	indent := strings.Repeat("  ", depth)
	if p.Handler != nil {
		log.Info("%vHandler: %v", indent, p.Handler.HelpShort)
	}
	log.Info("%vChildren:", indent)
	for k, v := range p.Children {
		log.Info("%v Pattern: %v", indent, k)
		v.dump(depth + 1)
	}
}
