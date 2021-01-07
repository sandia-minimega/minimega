// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package vlans

import (
	"strconv"
	"sync"
	"testing"
)

func TestAllocate(t *testing.T) {
	v := NewVLANs()

	for i := 0; i < 10; i++ {
		want, created, _ := v.Allocate("", strconv.Itoa(i))
		if !created {
			t.Errorf("VLAN already existed: %v", i)
		}

		got, created, _ := v.Allocate("", strconv.Itoa(i))
		if created {
			t.Errorf("VLAN doesn't exist: %v", i)
		}

		if got != want {
			t.Errorf("got: %v != want: %v", got, want)
		}
	}
}

func TestAllocateNamespace(t *testing.T) {
	v := NewVLANs()

	for i := 0; i < 10; i++ {
		got1, created, _ := v.Allocate("foo", strconv.Itoa(i))
		if !created {
			t.Errorf("VLAN already existed: %v", i)
		}

		got2, created, _ := v.Allocate("bar", strconv.Itoa(i))
		if !created {
			t.Errorf("VLAN already existed: %v", i)
		}

		if got1 == got2 {
			t.Errorf("got1: %v == got2: %v", got1, got2)
		}
	}
}

func TestAllocateOutOfVLANs(t *testing.T) {
	v := NewVLANs()

	var err error
	for i := 0; i < 4096; i++ {
		_, _, err = v.Allocate("", strconv.Itoa(i))
		if err != nil {
			break
		}
	}

	if err != ErrOutOfVLANs {
		t.Errorf("successfully allocated 4096 aliases...")
	}
}

func TestAllocateDeleteAllocate(t *testing.T) {
	v := NewVLANs()

	for i := 0; i < 10; i++ {
		alias := strconv.Itoa(i)

		if _, created, _ := v.Allocate("", alias); !created {
			t.Errorf("VLAN already existed: %v", i)
		}

		v.Delete("", "")

		if _, created, _ := v.Allocate("", alias); !created {
			t.Errorf("VLAN already existed: %v", i)
		}
	}
}

func TestAllocateDeleteAllocateNamespace(t *testing.T) {
	v := NewVLANs()

	for i := 0; i < 10; i++ {
		alias := strconv.Itoa(i)

		if _, created, _ := v.Allocate("foo", alias); !created {
			t.Errorf("VLAN already existed: %v", i)
		}
		if _, created, _ := v.Allocate("bar", alias); !created {
			t.Errorf("VLAN already existed: %v", i)
		}
	}

	// Delete all "foo" aliases
	v.Delete("foo", "")

	// Test that all the "foo" aliases get recreated and that the "bar" aliases
	// still exist
	for i := 0; i < 10; i++ {
		alias := strconv.Itoa(i)

		if _, created, _ := v.Allocate("foo", alias); !created {
			t.Errorf("VLAN already existed: %v", i)
		}
		if _, created, _ := v.Allocate("bar", alias); created {
			t.Errorf("VLAN didn't exist: %v", i)
		}
	}
}

func TestAllocateRange(t *testing.T) {
	v := NewVLANs()

	v.SetRange("", 0, 100)

	for i := 0; i < 100; i++ {
		alias := strconv.Itoa(i)

		if v, created, _ := v.Allocate("", alias); !created {
			t.Errorf("VLAN already existed: %v", i)
		} else if v < 0 || v >= 100 {
			t.Errorf("VLAN outside of specified bounds")
		}
	}
}

func TestAllocateRangeNamespace(t *testing.T) {
	v := NewVLANs()

	v.SetRange("foo", 0, 100)
	v.SetRange("bar", 100, 200)

	for i := 0; i < 100; i++ {
		alias := strconv.Itoa(i)

		if v, created, _ := v.Allocate("foo", alias); !created {
			t.Errorf("VLAN already existed: %v", i)
		} else if v < 0 || v >= 100 {
			t.Errorf("VLAN outside of specified bounds")
		}

		if v, created, _ := v.Allocate("bar", alias); !created {
			t.Errorf("VLAN already existed: %v", i)
		} else if v < 100 || v >= 200 {
			t.Errorf("VLAN outside of specified bounds")
		}
	}
}

func TestParallel(t *testing.T) {
	v := NewVLANs()

	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			// Append suffix to alias so that no aliases are a prefix of
			// another alias (otherwise Delete may delete them).
			alias := strconv.Itoa(i) + "net"

			vlan, created, err := v.Allocate("", alias)
			if !created {
				t.Errorf("VLAN already existed: %v", i)
				return
			} else if err != nil {
				t.Errorf("unable to allocate VLAN for %v: %v", alias, err)
				return
			}

			// make sure the mapping was set
			if got, _ := v.GetAlias(vlan); got.Value != alias {
				t.Errorf("got wrong alias for vlan: %v != %v", got, alias)
				return
			}
			if got, _ := v.GetVLAN("", alias); got != vlan {
				t.Errorf("got wrong vlan for alias: %v != %v", got, vlan)
				return
			}

			// delete the mapping
			v.Delete("", alias)

			// make sure the mapping is not set
			if got, _ := v.GetAlias(vlan); got.Value == alias {
				t.Errorf("found deleted alias %v by vlan", alias)
				return
			}
			if got, _ := v.GetVLAN("", alias); got == vlan {
				t.Errorf("found deleted alias %v by alias", alias)
				return
			}
		}(i)
	}

	wg.Wait()
}
