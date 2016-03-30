// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package vlans

import (
	"strconv"
	"testing"
)

func TestAllocate(t *testing.T) {
	v := NewAllocatedVLANs()

	for i := 0; i < 10; i++ {
		want, existed, _ := v.Allocate("", strconv.Itoa(i))
		if existed {
			t.Errorf("VLAN already existed: %v", i)
		}

		got, existed, _ := v.Allocate("", strconv.Itoa(i))
		if !existed {
			t.Errorf("VLAN doesn't exist: %v", i)
		}

		if got != want {
			t.Errorf("got: %v != want: %v", got, want)
		}
	}
}

func TestAllocateNamespace(t *testing.T) {
	v := NewAllocatedVLANs()

	for i := 0; i < 10; i++ {
		got1, existed, _ := v.Allocate("foo", strconv.Itoa(i))
		if existed {
			t.Errorf("VLAN already existed: %v", i)
		}

		got2, existed, _ := v.Allocate("bar", strconv.Itoa(i))
		if existed {
			t.Errorf("VLAN already existed: %v", i)
		}

		if got1 == got2 {
			t.Errorf("got1: %v == got2: %v", got1, got2)
		}
	}
}

func TestAllocateOutOfVLANs(t *testing.T) {
	v := NewAllocatedVLANs()

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
	v := NewAllocatedVLANs()

	for i := 0; i < 10; i++ {
		alias := strconv.Itoa(i)

		if _, existed, _ := v.Allocate("", alias); existed {
			t.Errorf("VLAN already existed: %v", i)
		}

		v.Delete("", alias)

		if _, existed, _ := v.Allocate("", alias); existed {
			t.Errorf("VLAN already existed: %v", i)
		}
	}
}

func TestAllocateDeleteAllocateNamespace(t *testing.T) {
	v := NewAllocatedVLANs()

	for i := 0; i < 10; i++ {
		alias := strconv.Itoa(i)

		if _, existed, _ := v.Allocate("foo", alias); existed {
			t.Errorf("VLAN already existed: %v", i)
		}
		if _, existed, _ := v.Allocate("bar", alias); existed {
			t.Errorf("VLAN already existed: %v", i)
		}
	}

	// Delete all "foo" aliases
	v.Delete("foo", "")

	// Test that all the "foo" aliases get recreated and that the "bar" aliases
	// still exist
	for i := 0; i < 10; i++ {
		alias := strconv.Itoa(i)

		if _, existed, _ := v.Allocate("foo", alias); existed {
			t.Errorf("VLAN already existed: %v", i)
		}
		if _, existed, _ := v.Allocate("bar", alias); !existed {
			t.Errorf("VLAN didn't exist: %v", i)
		}
	}
}

func TestAllocateRange(t *testing.T) {
	v := NewAllocatedVLANs()

	v.SetRange("", 0, 100)

	for i := 0; i < 100; i++ {
		alias := strconv.Itoa(i)

		if v, existed, _ := v.Allocate("", alias); existed {
			t.Errorf("VLAN already existed: %v", i)
		} else if v < 0 || v >= 100 {
			t.Errorf("VLAN outside of specified bounds")
		}
	}
}

func TestAllocateRangeNamespace(t *testing.T) {
	v := NewAllocatedVLANs()

	v.SetRange("foo", 0, 100)
	v.SetRange("bar", 100, 200)

	for i := 0; i < 100; i++ {
		alias := strconv.Itoa(i)

		if v, existed, _ := v.Allocate("foo", alias); existed {
			t.Errorf("VLAN already existed: %v", i)
		} else if v < 0 || v >= 100 {
			t.Errorf("VLAN outside of specified bounds")
		}

		if v, existed, _ := v.Allocate("bar", alias); existed {
			t.Errorf("VLAN already existed: %v", i)
		} else if v < 100 || v >= 200 {
			t.Errorf("VLAN outside of specified bounds")
		}
	}
}
