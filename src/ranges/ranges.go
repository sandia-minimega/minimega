package ranges

import (
	"errors"
//	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Range struct {
	Prefix	string
	Min		int
	Max		int
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

	// Make sure it's something like kn[1-50]
	match, err := regexp.MatchString(r.Prefix + "\\[.*\\]", s)
	if err != nil {
		return nil, err
	}
	if !match {
		if m2, err := regexp.MatchString(r.Prefix, s); m2 && err == nil {
			// assume they just handed us "kn1" or similar
			result = append(result, s)
			return result, nil
		} else {
			return nil, errors.New("Invalid range specification")
		}
	}

	// Get rid of the kn[] parts
	s = strings.Replace(s, r.Prefix+"[", "", 1)
	s = strings.Replace(s, "]", "", 1)

	parts := strings.Split(s, ",")

	for _, part := range parts {
		if strings.Contains(part, "-") {
			tmp, err := subrange(part)
			if err != nil {
				return nil, err
			}
			for _, n := range tmp {
				//result = append(result, r.Prefix + n)
				t, _ := strconv.Atoi(n)
				dedup[t] = t
			}
		} else {
			t, err := strconv.Atoi(part)
			if err != nil {return nil, err}
			//result = append(result, r.Prefix + t)
			dedup[t] = t
		}
	}

	var tmp []int
	for k, _ := range dedup {
		tmp = append(tmp, k)
	}

	sort.Ints(tmp)

	for _, n := range tmp {
		result = append(result, r.Prefix + strconv.Itoa(n))
	}

	return result, nil
}

func subrange(s string) ([]string, error) {
	limits := strings.Split(s, "-")
	start, err := strconv.Atoi(limits[0])
	if err != nil { return nil, err }
	end, err := strconv.Atoi(limits[1])
	if err != nil {return nil, err}

	var nodes []string
	for i := start; i <= end; i++ {
		nodes = append(nodes, strconv.Itoa(i))
	}

	return nodes, nil
}
