// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
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

func TestUnblacklist(t *testing.T) {
	v := NewVLANs()

	v.Blacklist(101)

	if err := v.Unblacklist(101); err != nil {
		t.Fatalf("unable to unblacklist: %v", err)
	}

	if got := v.GetBlacklist(); len(got) != 0 {
		t.Errorf("got blacklist %v, want empty", got)
	}

	if _, err := v.GetAlias(101); err != ErrUnallocated {
		t.Errorf("got error %v, want %v", err, ErrUnallocated)
	}

	// VLAN 101 is free again so we can add an alias for it
	if err := v.AddAlias("", "foo", 101); err != nil {
		t.Errorf("unable to add alias for unblacklisted VLAN: %v", err)
	}
}

func TestUnblacklistKeepsOtherEntries(t *testing.T) {
	v := NewVLANs()

	v.Blacklist(101)
	v.Blacklist(102)

	if _, _, err := v.Allocate("foo", "A"); err != nil {
		t.Fatal(err)
	}

	if err := v.Unblacklist(101); err != nil {
		t.Fatal(err)
	}

	if got := v.GetBlacklist(); len(got) != 1 || got[0] != 102 {
		t.Errorf("got blacklist %v, want [102]", got)
	}

	if got, err := v.GetVLAN("foo", "A"); err != nil || got != 103 {
		t.Errorf("got VLAN %v, %v for alias, want 103, nil", got, err)
	}
}

func TestUnblacklistNotBlacklisted(t *testing.T) {
	v := NewVLANs()

	if err := v.AddAlias("foo", "A", 200); err != nil {
		t.Fatal(err)
	}

	// The alias is literally named BLACKLISTED, it is not a blacklist entry
	if err := v.AddAlias("", BlacklistedVLAN, 300); err != nil {
		t.Fatal(err)
	}

	for _, vlan := range []int{100, 200, 300} {
		if err := v.Unblacklist(vlan); err == nil {
			t.Errorf("unblacklisted VLAN %v that is not blacklisted", vlan)
		}
	}

	if got, err := v.GetVLAN("foo", "A"); err != nil || got != 200 {
		t.Errorf("got VLAN %v, %v for alias, want 200, nil", got, err)
	}
	if got, err := v.GetVLAN("", BlacklistedVLAN); err != nil || got != 300 {
		t.Errorf("got VLAN %v, %v for alias, want 300, nil", got, err)
	}
}

func TestUnblacklistAllocate(t *testing.T) {
	v := NewVLANs()

	// VLAN 101 is the first VLAN that the allocator gives out
	if _, err := v.ParseVLAN("", "101"); err != nil {
		t.Fatal(err)
	}

	if got, _, err := v.Allocate("", "A"); err != nil || got != 102 {
		t.Fatalf("got VLAN %v, %v, want 102, nil", got, err)
	}

	if err := v.Unblacklist(101); err != nil {
		t.Fatal(err)
	}

	// The allocator must find the freed VLAN
	if got, _, err := v.Allocate("", "B"); err != nil || got != 101 {
		t.Errorf("got VLAN %v, %v, want 101, nil", got, err)
	}

	if got, _, err := v.Allocate("", "C"); err != nil || got != 103 {
		t.Errorf("got VLAN %v, %v, want 103, nil", got, err)
	}
}

func TestUnblacklistAllocateRange(t *testing.T) {
	v := NewVLANs()

	if err := v.SetRange("foo", 200, 210); err != nil {
		t.Fatal(err)
	}

	v.Blacklist(200)

	if got, _, err := v.Allocate("foo", "A"); err != nil || got != 201 {
		t.Fatalf("got VLAN %v, %v, want 201, nil", got, err)
	}

	// Fill the default range below the range for foo: 101 to 199, then 210
	for i := 0; i < 100; i++ {
		if _, _, err := v.Allocate("", strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}

	if err := v.Unblacklist(200); err != nil {
		t.Fatal(err)
	}

	// The default range must skip the range for foo
	if got, _, err := v.Allocate("", "X"); err != nil || got != 211 {
		t.Errorf("got VLAN %v, %v, want 211, nil", got, err)
	}

	if got, _, err := v.Allocate("foo", "B"); err != nil || got != 200 {
		t.Errorf("got VLAN %v, %v, want 200, nil", got, err)
	}
}

func TestHoldBlacklisted(t *testing.T) {
	v := NewVLANs()

	if err := v.AddAlias("foo", "A", 200); err != nil {
		t.Fatal(err)
	}

	for _, vlan := range []int{100, 200} {
		if _, err := v.HoldBlacklisted(vlan); err == nil {
			t.Errorf("held VLAN %v that is not blacklisted", vlan)
		}
	}

	v.Blacklist(101)

	release, err := v.HoldBlacklisted(101)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.HoldBlacklisted(101); err == nil {
		t.Error("held VLAN twice")
	}

	if _, err := v.ParseVLAN("foo", "101"); err == nil {
		t.Error("parsed VLAN that is held")
	}

	release()

	if got, err := v.ParseVLAN("foo", "101"); err != nil || got != 101 {
		t.Errorf("got VLAN %v, %v, want 101, nil", got, err)
	}
	if got := v.GetBlacklist(); len(got) != 1 || got[0] != 101 {
		t.Errorf("got blacklist %v, want [101]", got)
	}
}

func TestUnblacklistEndsHold(t *testing.T) {
	v := NewVLANs()

	v.Blacklist(101)

	if _, err := v.HoldBlacklisted(101); err != nil {
		t.Fatal(err)
	}
	if err := v.Unblacklist(101); err != nil {
		t.Fatal(err)
	}

	// VLAN 101 is free and not held so a user can blacklist it again
	if got, err := v.ParseVLAN("", "101"); err != nil || got != 101 {
		t.Errorf("got VLAN %v, %v, want 101, nil", got, err)
	}
	if _, err := v.HoldBlacklisted(101); err != nil {
		t.Errorf("unable to hold VLAN that is blacklisted again: %v", err)
	}
}

// TestReleaseKeepsNewerHold checks that the release function of an old hold
// does not end a newer hold on the same VLAN.
func TestReleaseKeepsNewerHold(t *testing.T) {
	v := NewVLANs()

	v.Blacklist(101)

	release1, err := v.HoldBlacklisted(101)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Unblacklist(101); err != nil {
		t.Fatal(err)
	}

	v.Blacklist(101)

	release2, err := v.HoldBlacklisted(101)
	if err != nil {
		t.Fatal(err)
	}

	release1()

	if _, err := v.ParseVLAN("", "101"); err == nil {
		t.Error("parsed VLAN after an old release ended the newer hold")
	}

	release2()

	if got, err := v.ParseVLAN("", "101"); err != nil || got != 101 {
		t.Errorf("got VLAN %v, %v, want 101, nil", got, err)
	}
}
