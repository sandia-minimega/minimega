// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vlans

import "testing"

func TestParseAlias(t *testing.T) {
	data := []struct {
		Alias     Alias
		Namespace string
		Value     string
	}{
		{
			Alias{"", "foo"},
			"",
			"foo",
		},
		{
			Alias{"bar", "foo"},
			"bar",
			"foo",
		},
		{
			Alias{"foo", "woo"},
			"bar",
			"foo//woo",
		},
	}

	for _, d := range data {
		got := ParseAlias(d.Namespace, d.Value)
		if got != d.Alias {
			t.Errorf("got: %v != want: %v", got, d.Alias)
		}
	}
}
