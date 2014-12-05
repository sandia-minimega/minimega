package minicli

import (
	"errors"
	"fmt"
)

// getHeader checks that all the header for all the responses are identical.
// If they are, it returns those header. Otherwise, returns an error.
func (r Responses) getHeader() (header []string, err error) {
	// Check to ensure that all the header are the same
	for i := 0; i < len(r)-1; i++ {
		// Assume that there's an error with these two hosts. Will clear before
		// returning if there's no error.
		err = fmt.Errorf("header mismatch, hosts: %s, %s", r[i].Host, r[i+1].Host)

		// Skip responses that have an error as we don't expect header for these.
		if r[i].Error != "" || r[i+1].Error != "" {
			continue
		}

		if r[i].Header != nil && r[i+1].Header != nil {
			// Both are not nil, check to make sure they are the same length
			if len(r[i].Header) != len(r[i+1].Header) {
				return
			}

			// Check to make sure all elements are the same
			for j := range r[i].Header {
				if r[i].Header[j] != r[i+1].Header[j] {
					return
				}
			}
		} else if r[i].Header == nil && r[i+1].Header == nil {
			// Both nil
			continue
		} else {
			// One but not both are nil => done goofed.
			return
		}
	}

	// Clear the error, we made it through the loop without returning
	err = nil

	// Find the first header that is non-nil to return
	for i := range r {
		if r[i].Error != "" && r[i].Header != nil {
			header = r[i].Header
			break
		}
	}

	return
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
