// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func hashPassword(t *testing.T, pw string) []byte {
	t.Helper()

	hashed, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hashing password: %v", err)
	}
	return hashed
}

func setPasswords(t *testing.T, entries []PasswordEntry) {
	t.Helper()

	orig := passwords
	passwords = entries
	t.Cleanup(func() {
		passwords = orig
	})
}

func TestPathMatches(t *testing.T) {
	cases := []struct {
		urlPath, entryPath string
		want               bool
	}{
		{"/", "/", true},
		{"/vm/fritz", "/", true},
		{"", "/", true},
		{"/vm", "/vm", true},
		{"/vm/", "/vm", true},
		{"/vm/fritz", "/vm", true},
		{"/vms", "/vm", false},
		{"/vmware", "/vm", false},
		{"/vm", "/vm/", true},
		{"/vm/", "/vm/", true},
		{"/vm/fritz", "/vm/", true},
		{"/vms", "/vm/", false},
		{"/other", "/vm", false},
	}

	for _, c := range cases {
		if got := pathMatches(c.urlPath, c.entryPath); got != c.want {
			t.Errorf("pathMatches(%q, %q) = %v, want %v", c.urlPath, c.entryPath, got, c.want)
		}
	}
}

func doRequest(h http.HandlerFunc, path, user, pass string, withAuth bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if withAuth {
		req.SetBasicAuth(user, pass)
	}

	rr := httptest.NewRecorder()
	mustAuth(h)(rr, req)
	return rr
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestMustAuthNoMatchingEntryPassesThrough(t *testing.T) {
	setPasswords(t, []PasswordEntry{
		{Path: "/admin", Username: "admin", Password: hashPassword(t, "adminpw")},
	})

	rr := doRequest(okHandler, "/public", "", "", false)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("expected no WWW-Authenticate header, got %q", rr.Header().Get("WWW-Authenticate"))
	}
}

func TestMustAuthUnauthenticatedRejected(t *testing.T) {
	setPasswords(t, []PasswordEntry{
		{Path: "/", Username: "admin", Password: hashPassword(t, "adminpw")},
	})

	rr := doRequest(okHandler, "/", "", "", false)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if rr.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("expected WWW-Authenticate header to be set")
	}
}

func TestMustAuthRecursiveAccessPreserved(t *testing.T) {
	setPasswords(t, []PasswordEntry{
		{Path: "/", Username: "admin", Password: hashPassword(t, "adminpw")},
		{Path: "/vm/fritz", Username: "fritz", Password: hashPassword(t, "fritzpw")},
	})

	// admin's broad "/" credentials must still authorize the narrower
	// "/vm/fritz" path -- this is the documented recursive-access behavior
	// that commit 102833b8 broke.
	rr := doRequest(okHandler, "/vm/fritz", "admin", "adminpw", true)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected admin to access /vm/fritz, got %d", rr.Code)
	}
}

func TestMustAuthNarrowUserIsolation(t *testing.T) {
	setPasswords(t, []PasswordEntry{
		{Path: "/", Username: "admin", Password: hashPassword(t, "adminpw")},
		{Path: "/vm/fritz", Username: "fritz", Password: hashPassword(t, "fritzpw")},
		{Path: "/vm/john", Username: "john", Password: hashPassword(t, "johnpw")},
	})

	if rr := doRequest(okHandler, "/vm/john", "fritz", "fritzpw", true); rr.Code != http.StatusUnauthorized {
		t.Errorf("expected fritz denied access to /vm/john, got %d", rr.Code)
	}
	if rr := doRequest(okHandler, "/", "fritz", "fritzpw", true); rr.Code != http.StatusUnauthorized {
		t.Errorf("expected fritz denied access to /, got %d", rr.Code)
	}
}

func TestMustAuthSegmentBoundaryFix(t *testing.T) {
	setPasswords(t, []PasswordEntry{
		{Path: "/vm", Username: "vmuser", Password: hashPassword(t, "vmpw")},
	})

	// "/vms" only shares a string prefix with "/vm" -- it must not be
	// covered by the "/vm" rule.
	if rr := doRequest(okHandler, "/vms", "", "", false); rr.Code != http.StatusOK {
		t.Errorf("expected /vms to be unprotected by /vm rule, got %d", rr.Code)
	}

	// but a real sub-path of "/vm" is still protected.
	if rr := doRequest(okHandler, "/vm/anything", "", "", false); rr.Code != http.StatusUnauthorized {
		t.Errorf("expected /vm/anything to require auth, got %d", rr.Code)
	}
	if rr := doRequest(okHandler, "/vm/anything", "vmuser", "vmpw", true); rr.Code != http.StatusOK {
		t.Errorf("expected vmuser to access /vm/anything, got %d", rr.Code)
	}
}

func TestMustAuthTrailingSlashEquivalence(t *testing.T) {
	setPasswords(t, []PasswordEntry{
		{Path: "/vm/", Username: "vmuser", Password: hashPassword(t, "vmpw")},
	})

	if rr := doRequest(okHandler, "/vm", "vmuser", "vmpw", true); rr.Code != http.StatusOK {
		t.Errorf("expected vmuser to access /vm, got %d", rr.Code)
	}
	if rr := doRequest(okHandler, "/vm/sub", "vmuser", "vmpw", true); rr.Code != http.StatusOK {
		t.Errorf("expected vmuser to access /vm/sub, got %d", rr.Code)
	}
}
