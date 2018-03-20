package main

import (
	"testing"
)

func TestParseDuration(t *testing.T) {
	// zero: should parse
	duration, err := parseDuration("0")
	if duration != 0 {
		t.Error("expected duration = 0")
	}

	// non-zero: should parse
	duration, _ = parseDuration("60")
	if duration != 60 {
		t.Error("expected duration = 60")
	}

	// fractional input: should NOT parse (takes int)
	_, err = parseDuration("0.5")
	if err == nil {
		t.Error("expected err != nil)")
	}

	// zero usage of "d" suffix: should parse
	duration, _ = parseDuration("0d")
	if duration != 0 {
		t.Error("expected duration = 0")
	}

	// non-zero usage of "d" suffix: should parse
	duration, _ = parseDuration("2d")
	if duration != 2*24*60 {
		t.Error("expected duration = 2880")
	}

	// non-zero usage of suffix supported by time.ParseDuration(): should parse
	duration, _ = parseDuration("2h")
	if duration != 2*60 {
		t.Error("expected duration = 120, got", duration)
	}

	// non-zero usage of unsupported suffix: should NOT parse
	_, err = parseDuration("2g")
	if err == nil {
		t.Error("expected err != nil, got", err)
	}

	// negative time: should parse
	duration, err = parseDuration("-2")
	if duration != -2 {
		t.Error("expected duration = -2, got ", duration)
	}

	// random string/garbage: should NOT parse
	_, err = parseDuration("%wjfe")
	if err == nil {
		t.Error("expected err != nil")
	}
}
