package minicli

import (
	"errors"
	"fmt"
)

// getHeader checks that all the header for all the responses are identical.
// If they are, it returns those header. Otherwise, returns an error.
func (r Responses) getHeader() ([]string, error) {
	var host string
	var header []string

	// Find the first header that is non-nil
	for _, x := range r {
		if x.Error == "" && x.Header != nil {
			host = x.Host
			header = x.Header
			break
		}
	}

	if header == nil {
		return nil, nil
	}

	// Check to ensure that all the header are the same
	for _, x := range r {
		// Don't check headers for responses with an error.
		if x.Error != "" {
			continue
		}

		// Prebuild an error with these two hosts.
		err := fmt.Errorf("header mismatch, hosts: %s, %s", host, x.Host)

		if x.Header == nil || len(header) != len(x.Header) {
			return nil, err
		}

		// Check to make sure all elements are the same
		for i, h := range header {
			if h != x.Header[i] {
				return nil, err
			}
		}
	}

	return header, nil
}

// validTabular checks whether all the responses have tabular data and whether
// the width of the tabular data matches the width of the headers. Generates an
// error if there is a mixture of simple responses and tabular data or if there
// are width mismatches.
func (r Responses) validTabular(header []string) (bool, error) {
	var simple, tabular bool
	for i := range r {
		// Ignore responses with an error
		if r[i].Error != "" {
			continue
		}

		if r[i].Tabular != nil {
			// Ignore the simple response if there's tabular data
			tabular = true

			if header != nil {
				// Check the width matches
				for j := range r[i].Tabular {
					if len(r[i].Tabular[j]) != len(header) {
						err := fmt.Errorf("tabular width mismatch, host: %s, row: %d", r[i].Host, j)
						return false, err
					}
				}
			}
		} else {
			simple = true
		}
	}

	if simple && tabular {
		return false, errors.New("responses mix simple and tabular data")
	}

	return tabular, nil
}
