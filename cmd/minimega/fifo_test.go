package main

import "testing"

func TestByteFifoKeepsExactlyLimit(t *testing.T) {
	bf := NewByteFifo(4)
	bf.Write([]byte("abcdefgh"))
	if got := string(bf.Get()); got != "efgh" {
		t.Fatalf("got %q, want %q", got, "efgh")
	}
	bf.Write([]byte("ij"))
	if got := string(bf.Get()); got != "ghij" {
		t.Fatalf("got %q, want %q", got, "ghij")
	}
	z := NewByteFifo(0)
	z.Write([]byte("x"))
	if len(z.Get()) != 0 {
		t.Fatalf("limit 0 kept %q", z.Get())
	}
}
