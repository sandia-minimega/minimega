// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// ranges provides methods to expand or condense ranges of like strings. This
// is used for specifying a list of hosts eg. host[1-10]. The ranges package
// can expand host[1-10] into a slice of strings: [host1, host2, host3...].
// Similarly, it can condense a slice of strings into a compact form: [host1,
// host2, host5, host6] -> host[1-2,5-6].
package ranges

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Range struct {
	Prefix string
	Min    int
	Max    int
}

func NewRange(prefix string, min, max int) (*Range, error) {
	if min > max {
		return nil, errors.New("invalid range: min > max")
	}
	r := &Range{prefix, min, max}
	return r, nil
}

func (r *Range) SplitRange(s string) ([]string, error) {
	var result []string
	dedup := make(map[int]int)

	if !strings.HasPrefix(s, r.Prefix) {
		return nil, errors.New("invalid range specification")
	}

	s = strings.TrimPrefix(s, r.Prefix)

	if strings.HasPrefix(s, "[") && !strings.HasSuffix(s, "]") {
		return nil, errors.New("invalid range specification")
	}

	if !strings.HasPrefix(s, "[") {
		// assume they just handed us "kn1" or similar
		if _, err := strconv.Atoi(s); err != nil {
			return nil, errors.New("invalid range specification")
		}

		return []string{r.Prefix + s}, nil
	}

	// Must be a range like "kn[1-50]" (without prefix at this point)
	parts := strings.Split(s[1:len(s)-1], ",")

	pad := -1
	for _, part := range parts {
		if strings.Contains(part, "-") {
			tmp, err := subrange(part)
			if err != nil {
				return nil, err
			}
			for _, n := range tmp {
				if pad == -1 {
					pad = len(n)
				} else if len(n) != pad {
					pad = 0
				}
				t, _ := strconv.Atoi(n)
				if t < r.Min || t > r.Max {
					return nil, fmt.Errorf("value of out range: %v", t)
				}
				dedup[t] = t
			}
		} else {
			if pad == -1 {
				pad = len(part)
			} else if len(part) != pad {
				pad = 0
			}
			t, err := strconv.Atoi(part)
			if err != nil {
				return nil, err
			}
			if t < r.Min || t > r.Max {
				return nil, fmt.Errorf("value of out range: %v", t)
			}
			dedup[t] = t
		}
	}

	var tmp []int
	for k, _ := range dedup {
		tmp = append(tmp, k)
	}

	sort.Ints(tmp)

	for _, n := range tmp {
		format := "%d"
		if pad != 0 {
			format = "%0" + fmt.Sprintf("%v", pad) + "d"
		}
		name := fmt.Sprintf(format, n)
		result = append(result, r.Prefix+name)
	}

	return result, nil
}

// SplitList takes a string such as "foo,bar[1-3]" and expands it to a fully
// enumerated list of names.
func SplitList(in string) ([]string, error) {
	var res, parts []string

	var prev int
	var inside bool

	for i := 0; i < len(in); i++ {
		if in[i] == '[' {
			if inside {
				return nil, fmt.Errorf("nested '[' at char %d", i)
			} else {
				inside = true
			}
		} else if in[i] == ']' {
			if inside {
				inside = false
			} else {
				return nil, fmt.Errorf("unmatched ']' at char %d", i)
			}
		} else if in[i] == ',' {
			if !inside {
				parts = append(parts, in[prev:i])
				prev = i + 1
			}
		}
	}

	// handle last entry on the line and look for unterminated ranges
	if inside {
		return nil, errors.New("unterminated '['")
	} else if prev < len(in) {
		parts = append(parts, in[prev:])
	}

	for _, v := range parts {
		index := strings.IndexRune(v, '[')
		if index == -1 {
			res = append(res, v)
			continue
		}

		prefix := v[:index]
		r, _ := NewRange(prefix, 0, math.MaxInt32)
		ret, err := r.SplitRange(v)
		if err != nil {
			return nil, err
		}
		res = append(res, ret...)
	}

	return res, nil
}

// UnsplitList takes a list of strings like ["foo1.bar", "foo2.bar"] and
// condenses them to "foo[1-2].bar".
func UnsplitList(vals []string) string {
	trie := newTrie()
	for _, v := range vals {
		trie.Add(v)
	}

	return strings.Join(trie.Flatten(), ",")
}

// Turn an array of node names into a single string like kn[1-5,20]
func (r *Range) UnsplitRange(names []string) (string, error) {
	var nums []int

	// Remove the prefix from every name and put into an array of ints
	for _, s := range names {
		if !strings.HasPrefix(s, r.Prefix) {
			return "", fmt.Errorf("invalid name: %v (expected prefix %v)", s, r.Prefix)
		}

		if i, err := strconv.Atoi(strings.TrimPrefix(s, r.Prefix)); err == nil {
			nums = append(nums, i)
		} else {
			return "", fmt.Errorf("invalid name: %v (expected numbers after prefix)", s)
		}
	}

	if len(nums) == 0 {
		return "", errors.New("nothing to parse")
	}

	return r.Prefix + unsplitInts(nums), nil
}

// unsplitInts takes ints as a slice (e.g. [1,2,3,5]) and turns them into a
// string (e.g. [1-3,5]).
func unsplitInts(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	if len(nums) == 1 {
		return strconv.Itoa(nums[0])
	}

	// Sort the numbers
	sort.Ints(nums)

	// "count along" to find stretches like 1-5
	result := "[" + strconv.Itoa(nums[0])
	start := nums[0]
	prev := nums[0]
	for i := 1; i < len(nums); i++ {
		if nums[i]-prev != 1 {
			if start != prev {
				result = result + "-" + strconv.Itoa(prev) + "," + strconv.Itoa(nums[i])
			} else {
				result = result + "," + strconv.Itoa(nums[i])
			}
			start = nums[i]
		} else if i == len(nums)-1 {
			if nums[i]-prev == 1 {
				result = result + "-" + strconv.Itoa(nums[i])
			} else {
				result = result + "," + strconv.Itoa(nums[i])
			}
		}
		prev = nums[i]
	}
	result = result + "]"

	return result
}

func subrange(s string) ([]string, error) {
	limits := strings.Split(s, "-")
	if len(limits) != 2 {
		return nil, fmt.Errorf("invalid subrange %v", s)
	}

	// check for subrange padding
	pad := 0
	if len(limits[0]) == len(limits[1]) {
		pad = len(limits[0])
	}

	start, err := strconv.Atoi(limits[0])
	if err != nil {
		return nil, err
	}
	end, err := strconv.Atoi(limits[1])
	if err != nil {
		return nil, err
	}

	var nodes []string
	for i := start; i <= end; i++ {
		format := "%d"
		if pad != 0 {
			format = "%0" + fmt.Sprintf("%v", pad) + "d"
		}
		name := fmt.Sprintf(format, i)

		nodes = append(nodes, name)
	}

	return nodes, nil
}

func (r *Range) RangeToInts(names []string) []int {
	var nums []int

	// Remove the prefix from every name and put into an array of ints
	for _, s := range names {
		if !strings.HasPrefix(s, r.Prefix) {
			return []int{}
		}

		if i, err := strconv.Atoi(strings.TrimPrefix(s, r.Prefix)); err == nil {
			nums = append(nums, i)
		} else {
			return []int{}
		}
	}

	if len(nums) == 0 {
		return []int{}
	}

	// Sort the numbers
	sort.Ints(nums)

	return nums
}
