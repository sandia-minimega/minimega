// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"minicli"
	"regexp"
	"strconv"
	"strings"
)

type Command struct {
	Name string
	Help string

	// Args is the combination of all variants
	Args []Arg

	Variants []*Variant
}

type Variant struct {
	Pattern  string
	Template string

	Args         []Arg
	RequiredArgs []Arg
}

type Arg struct {
	Name     string
	Optional bool
	Options  []string
}

var weirdChars = regexp.MustCompile("[-:/]")

func NewVariant(pattern []minicli.PatternItem) *Variant {
	v := &Variant{
		Pattern: minicli.PatternItems(pattern).String(),
	}

	template := []string{}

	for _, item := range pattern {
		arg := Arg{
			Optional: item.IsOptional(),
		}

		switch {
		case item.IsLiteral():
			template = append(template, strconv.Quote(item.Text))
		case item.IsChoice() && !item.IsOptional() && len(item.Options) == 1:
			template = append(template, strconv.Quote(item.Options[0]))
		case item.IsChoice():
			arg.Name = strings.Join(item.Options, "_or_")
			arg.Options = item.Options
		default:
			arg.Name = item.Key
		}

		if arg.Name != "" {
			// clean up any weird characters that might be in the name
			arg.Name = weirdChars.ReplaceAllString(arg.Name, "_")

			v.Args = append(v.Args, arg)
			template = append(template, arg.Name)

			if !arg.Optional {
				v.RequiredArgs = append(v.RequiredArgs, arg)
			}
		}
	}

	v.Template = strings.Join(template, ", ")

	return v
}
