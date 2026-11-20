// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import "testing"

func TestMigrateProgress(t *testing.T) {
	ram := func(total, transferred interface{}) map[string]interface{} {
		return map[string]interface{}{"total": total, "transferred": transferred}
	}

	tests := []struct {
		name     string
		in       map[string]interface{}
		status   string
		complete float64
		wantErr  bool
	}{
		{"no migration", map[string]interface{}{}, "", 0, false},
		{"setup", map[string]interface{}{"status": "setup"}, "setup", 0, false},
		{"device", map[string]interface{}{"status": "device"}, "device", 0, false},
		{"cancelled", map[string]interface{}{"status": "cancelled"}, "cancelled", 0, false},
		{"failed", map[string]interface{}{"status": "failed"}, "failed", 0, false},
		{"completed", map[string]interface{}{"status": "completed"}, "completed", 100, false},
		{"active", map[string]interface{}{"status": "active", "ram": ram(200.0, 50.0)}, "active", 25, false},
		{"active without ram", map[string]interface{}{"status": "active"}, "active", 0, true},
		{"active with bad ram", map[string]interface{}{"status": "active", "ram": "x"}, "active", 0, true},
		{"active without total", map[string]interface{}{"status": "active", "ram": map[string]interface{}{"transferred": 1.0}}, "active", 0, true},
		{"active without transferred", map[string]interface{}{"status": "active", "ram": map[string]interface{}{"total": 1.0}}, "active", 0, true},
		{"active with zero total", map[string]interface{}{"status": "active", "ram": ram(0.0, 0.0)}, "active", 0, true},
		{"non-string status", map[string]interface{}{"status": 1.0}, "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, complete, err := migrateProgress(tt.in)
			if status != tt.status || complete != tt.complete || (err != nil) != tt.wantErr {
				t.Errorf("migrateProgress(%v) = %q, %v, %v; want %q, %v, err=%v", tt.in, status, complete, err, tt.status, tt.complete, tt.wantErr)
			}
		})
	}
}
