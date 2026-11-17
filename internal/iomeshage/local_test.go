package iomeshage

import (
	"os"
	"path/filepath"
	"testing"
)

// Deleting "/" must empty the served base directory but keep it.
func TestDeleteBaseKeepsDirectory(t *testing.T) {
	base := t.TempDir()
	iom, err := New(base, nil, "", false)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := iom.Delete("/"); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(base)
	if err != nil || !fi.IsDir() {
		t.Fatalf("base directory was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("contents not removed: %v", err)
	}
}
