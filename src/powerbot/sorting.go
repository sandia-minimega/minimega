// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

// Do some nice sorting
package main

import (
	"strconv"
	"unicode"
)

type ByNumber []string

func findNum(s string) int {
	n := ""
	for _, element := range s {
		if unicode.IsNumber(element) {
			n = n + string(element)
		}
	}
	num, _ := strconv.Atoi(n)
	return num
}

func (s ByNumber) Len() int {
	return len(s)
}

func (s ByNumber) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s ByNumber) Less(i, j int) bool {
	return findNum(s[i]) < findNum(s[j])
}
