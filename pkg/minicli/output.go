// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minicli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/sandia-minimega/minimega/v2/pkg/ranges"
)

type table [][]string

func (t table) Len() int {
	return len(t)
}

func (t table) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t table) Less(i, j int) bool {
	for k := 0; k < len(t[i]) && k < len(t[j]); k++ {
		if t[i][k] != t[j][k] {
			// If both convert to ints, compare using int comparison
			v, err := strconv.Atoi(t[i][k])
			v2, err2 := strconv.Atoi(t[j][k])
			if err == nil && err2 == nil {
				return v < v2
			}

			return t[i][k] < t[j][k]
		}
	}

	return true
}

// Return a string representation using the current output mode
func (r Responses) String() string {
	if len(r) == 0 {
		return ""
	}

	// Copy the global settings where the overrides are not set
	if r[0].Flags == nil {
		r[0].Flags = new(Flags)
		*r[0].Flags = defaultFlags
	}

	if r.json() {
		bytes, err := json.Marshal(r)
		if err != nil {
			// TODO: Should this be JSON-formatted too?
			return err.Error()
		}

		return string(bytes)
	}

	header, err := r.getHeader()
	if err != nil {
		return err.Error()
	}

	// TODO: What is Header for simple responses?

	tabular, err := r.validTabular(header)
	if err != nil {
		return err.Error()
	}

	var count int
	for _, x := range r {
		count += len(x.Tabular)
	}

	var buf bytes.Buffer

	if tabular && count > 0 {
		r.tabularString(&buf, header)
	} else if r.compress() && len(r) > 1 {
		r.compressString(&buf)
	} else {
		for _, v := range r {
			if v.Error == "" && v.Response != "" {
				if r.annotate() {
					buf.WriteString(v.Host)
					buf.WriteString(": ")
				}
				buf.WriteString(v.Response)
				buf.WriteString("\n")
			}
		}
	}

	resp := buf.String()
	return strings.TrimSpace(resp)
}

// Error returns a string containing all the errors in the responses
func (r Responses) Error() string {
	var buf bytes.Buffer

	// Append errors from hosts
	for i, v := range r {
		if v.Error != "" {
			fmt.Fprintf(&buf, "Error (%s): %s", v.Host, v.Error)

			// add a newline unless this is our last iteration
			if i != len(r)-1 {
				io.WriteString(&buf, "\n")
			}
		}
	}

	return strings.TrimSpace(buf.String())
}

func (r Responses) tabularString(buf io.Writer, header []string) {
	annotate := r.annotate()

	// Add extra header for host if annotate is set
	if annotate {
		header = append([]string{"host"}, header...)
	}

	// Collect all the tabular data, adding extra data column for host, if
	// annotate is set.
	data := [][]string{}
	for _, v := range r {
		for _, row := range v.Tabular {
			if annotate {
				row = append([]string{v.Host}, row...)
			}

			data = append(data, row)
		}
	}

	if r.sort() {
		sort.Sort(table(data))
	}

	if r.csv() {
		r.printCSV(buf, header, data)
	} else {
		r.printTabular(buf, header, data)
	}
}

func (r Responses) compressString(buf io.Writer) {
	buckets := map[uint64][]*Response{}

	// Find responses that have the same output by hashing them into buckets
	for _, v := range r {
		if v.Error == "" && v.Response != "" {
			h := fnv.New64a()
			h.Write([]byte(v.Response))
			k := h.Sum64()

			buckets[k] = append(buckets[k], v)
		}
	}

	if len(buckets) == 0 {
		return
	}

	// Compress hostnames into ranges
	res := map[string]uint64{}
	for hash, resps := range buckets {
		if len(resps) == 1 {
			res[resps[0].Host] = hash
			continue
		}

		hosts := []string{}
		for _, r := range resps {
			hosts = append(hosts, r.Host)
		}

		res[ranges.UnsplitList(hosts)] = hash
	}

	// Sort the keys of ranges
	hosts := []string{}
	for k := range res {
		hosts = append(hosts, k)
	}
	sort.Strings(hosts)

	for _, h := range hosts {
		resp := buckets[res[h]][0]

		if r.annotate() {
			buf.Write([]byte(h))
			buf.Write([]byte(": "))
		}

		buf.Write([]byte(resp.Response))
		buf.Write([]byte("\n"))
	}
}

func (r Responses) printTabular(buf io.Writer, header []string, data [][]string) {
	w := new(tabwriter.Writer)
	w.Init(buf, 5, 0, 1, ' ', 0)
	defer w.Flush()

	if r.headers() {
		for i, h := range header {
			if i != 0 {
				io.WriteString(w, "\t| ")
			}
			io.WriteString(w, h)
		}
		io.WriteString(w, "\n")
	}

	for _, row := range data {
		for i, v := range row {
			if i != 0 {
				io.WriteString(w, "\t| ")
			}
			io.WriteString(w, v)
		}
		io.WriteString(w, "\n")
	}
}

func (r Responses) printCSV(buf io.Writer, header []string, data [][]string) {
	w := csv.NewWriter(buf)
	defer w.Flush()

	if r.headers() {
		w.Write(header)
	}

	w.WriteAll(data)
}
