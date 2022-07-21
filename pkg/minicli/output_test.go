// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minicli

import (
	"sort"
	"strconv"
	"testing"
)

// sortData is in a random order and the third column is the correct order.
var sortData = [][]string{
	[]string{"b", "b", "2"},
	[]string{"a", "a", "0"},
	[]string{"d", "a", "4"},
	[]string{"a", "b", "1"},
	[]string{"e", "a", "5"},
	[]string{"c", "a", "3"},
}

// sortDataInts is in a random order and the third column is the correct order.
var sortDataInts = [][]string{
	[]string{"2", "b", "2"},
	[]string{"0", "a", "0"},
	[]string{"4", "a", "4"},
	[]string{"1", "b", "1"},
	[]string{"5", "a", "5"},
	[]string{"3", "a", "3"},
}

func TestSort(t *testing.T) {
	data := make([][]string, len(sortData))
	copy(data, sortData)

	sort.Sort(table(data))

	for i := 0; i < len(data); i++ {
		v, _ := strconv.Atoi(data[i][2])
		if i != v {
			t.Errorf("out of order, %v: %v", i, data)
		}
	}
}

func TestSortInts(t *testing.T) {
	data := make([][]string, len(sortDataInts))
	copy(data, sortData)

	sort.Sort(table(data))

	for i := 0; i < len(data); i++ {
		v, _ := strconv.Atoi(data[i][2])
		if i != v {
			t.Errorf("out of order, %v: %v", i, data)
		}
	}
}
