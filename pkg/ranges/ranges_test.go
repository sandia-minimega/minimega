// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ranges

import (
	"fmt"
	"math/rand"
	"testing"
)

func shuffle(vals []string) {
	r := rand.New(rand.NewSource(0))
	for i := len(vals) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		vals[i], vals[j] = vals[j], vals[i]
	}
}

func TestSplitRangeless(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	expected := []string{"kn1"}
	input := "kn1"

	res, _ := r.SplitRange(input)

	es := fmt.Sprintf("%v", expected)
	rs := fmt.Sprintf("%v", res)

	if es != rs {
		t.Fatal("SplitRange returned: ", res, ", expected: ", expected)
	}
}

func TestSplitRange(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	expected := []string{"kn1", "kn2", "kn3", "kn100"}
	input := "kn[1-3,100]"

	res, _ := r.SplitRange(input)

	es := fmt.Sprintf("%v", expected)
	rs := fmt.Sprintf("%v", res)

	if es != rs {
		t.Fatal("SplitRange returned: ", res, ", expected: ", expected)
	}
}

func TestSplitRangeNoPrefix(t *testing.T) {
	r, _ := NewRange("", 1, 520)

	expected := []string{"1", "2", "3", "100"}
	input := "[1-3,100]"

	res, _ := r.SplitRange(input)

	es := fmt.Sprintf("%v", expected)
	rs := fmt.Sprintf("%v", res)

	if es != rs {
		t.Fatal("SplitRange returned: ", res, ", expected: ", expected)
	}
}

func TestSplitRangePadded(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	expected := []string{"kn008", "kn009", "kn010", "kn011", "kn100"}
	input := "kn[008-011,100]"

	res, _ := r.SplitRange(input)

	es := fmt.Sprintf("%v", expected)
	rs := fmt.Sprintf("%v", res)

	if es != rs {
		t.Fatal("SplitRangePadded returned: ", res, ", expected: ", expected)
	}
}

func TestUnsplitRange(t *testing.T) {
	r, _ := NewRange("kn", 1, 520)

	expected := "kn[1-5]"
	input := []string{"kn1", "kn2", "kn3", "kn4", "kn5"}

	res, err := r.UnsplitRange(input)
	if err != nil {
		t.Fatal("UnsplitRange returned error: ", err)
	}
	if expected != res {
		t.Fatal("UnsplitRange returned: ", res)
	}

	expected = "kn[1-5,20]"
	input = []string{"kn1", "kn2", "kn3", "kn4", "kn5", "kn20"}

	res, err = r.UnsplitRange(input)
	if err != nil {
		t.Fatal("UnsplitRange returned error: ", err)
	}
	if expected != res {
		t.Fatal("UnsplitRange returned: ", res)
	}

	expected = "kn[1-5,20,44-45]"
	input = []string{"kn44", "kn45", "kn1", "kn2", "kn3", "kn4", "kn5", "kn20"}

	res, err = r.UnsplitRange(input)
	if err != nil {
		t.Fatal("UnsplitRange returned error: ", err)
	}
	if expected != res {
		t.Fatal("UnsplitRange returned: ", res)
	}
}

func TestSplitList(t *testing.T) {
	data := []struct {
		input string
		count int
	}{
		{"f", 1},
		{"0", 1},
		{"10", 1},
		{"foo", 1},
		{"foo,", 1},
		{"foo,bar", 2},
		{"foo,bar[0-1]", 3},
		{"foo,bar[0-1],kn[1,2,3]", 6},
	}

	for _, v := range data {
		res, err := SplitList(v.input)
		if err != nil {
			t.Errorf("expand `%s` -- %v", v.input, err)
		} else if len(res) != v.count {
			t.Errorf("want %d, got %v", v.count, res)
		} else {
			t.Logf("got: %v", res)
		}
	}
}

func TestUnsplitList(t *testing.T) {
	hosts := []string{}

	for i := 0; i < 10; i++ {
		hosts = append(hosts, fmt.Sprintf("node%d", i))
		hosts = append(hosts, fmt.Sprintf("n%d", i))
		hosts = append(hosts, fmt.Sprintf("foo%d", i))
	}

	want := "foo[0-9],node[0-9],n[0-9]"
	got := UnsplitList(hosts)

	if want != got {
		t.Errorf("got: `%s`, want `%s`", got, want)
	}
}

func TestUnsplitListSuffix(t *testing.T) {
	hosts := []string{}

	for i := 0; i < 5; i++ {
		hosts = append(hosts, fmt.Sprintf("foo%d.bar1", i))
		hosts = append(hosts, fmt.Sprintf("foo%d.bar2", i))
	}
	for i := 0; i < 2; i++ {
		hosts = append(hosts, fmt.Sprintf("foo%d.bar3", i))
	}
	for i := 0; i < 3; i++ {
		hosts = append(hosts, fmt.Sprintf("foo%d.bar5", i))
	}
	for i := 0; i < 5; i++ {
		hosts = append(hosts, fmt.Sprintf("foo%d.car", i))
	}

	shuffle(hosts)

	want := "foo2.bar[1-2,5],foo[3-4].bar[1-2],foo[0-1].bar[1-3,5],foo[0-4].car"
	got := UnsplitList(hosts)

	if want != got {
		t.Errorf("got: `%v`, want `%v`", got, want)
	}
}

func TestUnsplitListSuffix2(t *testing.T) {
	hosts := []string{
		"foo1.10g",
		"foo2.10g",
		"foo3.100g",
		"foo4.100g",
		"foo5.1000g",
		"foo6.1000g",
	}

	shuffle(hosts)

	want := "foo[5-6].1000g,foo[3-4].100g,foo[1-2].10g"
	got := UnsplitList(hosts)

	if want != got {
		t.Errorf("got: `%v`, want `%v`", got, want)
	}
}

func TestUnsplitListSkip(t *testing.T) {
	hosts := []string{}

	for i := 0; i < 10; i++ {
		if i != 5 {
			hosts = append(hosts, fmt.Sprintf("n%d", i))
		}
		if i != 9 {
			hosts = append(hosts, fmt.Sprintf("node%d", i))
		}
	}

	want := "node[0-8],n[0-4,6-9]"
	got := UnsplitList(hosts)

	if want != got {
		t.Errorf("got: `%s`, want `%s`", got, want)
	}
}
