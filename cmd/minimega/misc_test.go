// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"math"
	"strconv"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestHasCommand(t *testing.T) {
	// Make some dummy commands with nested subcommands
	strs := []string{"foo", "bar", "baz"}
	cmds := []*minicli.Command{}
	for i, str := range strs {
		var sub *minicli.Command
		if i != 0 {
			sub = cmds[i-1]
		}

		cmds = append(cmds, &minicli.Command{Original: str, Subcommand: sub})
	}

	for i := range cmds {
		// Test where we know should have command
		for j := len(cmds) - 1; j >= i; j-- {
			if !hasCommand(cmds[j], strs[i]) {
				t.Errorf("expected cmd %d to have `%v`", j, strs[i])
			}
		}

		// Test where we know we should *not* have command
		for j := 0; j < i; j++ {
			if hasCommand(cmds[j], strs[i]) {
				t.Errorf("expected cmd %d not to have `%v`", j, strs[i])
			}
		}
	}
}

func testMesh(size int, pairwise bool, t *testing.T) {
	var vals []string
	for i := 0; i < size; i++ {
		vals = append(vals, strconv.Itoa(i))
	}

	res := mesh(vals, pairwise)
	if len(res) != len(vals) {
		t.Errorf("expected slice for each val, got %v", len(res))
	}

	want := len(vals) - 1
	if !pairwise {
		// should be ceil(log2(vals))
		want = int(math.Ceil(math.Log2(float64(len(vals)))))
	}

	for k, vals2 := range res {
		if len(vals2) != want {
			t.Errorf("expected %v values in slice for %v, got %v", want, k, len(vals2))
		}

		for _, v := range vals2 {
			if k == v {
				t.Errorf("contains self-loop: %v -> %v", k, v)
			}
		}
	}
}

func TestMesh2(t *testing.T)  { testMesh(2, false, t) }
func TestMesh3(t *testing.T)  { testMesh(3, false, t) }
func TestMesh4(t *testing.T)  { testMesh(4, false, t) }
func TestMesh5(t *testing.T)  { testMesh(5, false, t) }
func TestMesh6(t *testing.T)  { testMesh(6, false, t) }
func TestMesh7(t *testing.T)  { testMesh(7, false, t) }
func TestMesh8(t *testing.T)  { testMesh(8, false, t) }
func TestMesh9(t *testing.T)  { testMesh(9, false, t) }
func TestMesh20(t *testing.T) { testMesh(20, false, t) }
func TestMesh50(t *testing.T) { testMesh(50, false, t) }

func TestMeshPairwise2(t *testing.T)  { testMesh(2, true, t) }
func TestMeshPairwise3(t *testing.T)  { testMesh(3, true, t) }
func TestMeshPairwise4(t *testing.T)  { testMesh(4, true, t) }
func TestMeshPairwise5(t *testing.T)  { testMesh(5, true, t) }
func TestMeshPairwise6(t *testing.T)  { testMesh(6, true, t) }
func TestMeshPairwise7(t *testing.T)  { testMesh(7, true, t) }
func TestMeshPairwise8(t *testing.T)  { testMesh(8, true, t) }
func TestMeshPairwise9(t *testing.T)  { testMesh(9, true, t) }
func TestMeshPairwise20(t *testing.T) { testMesh(20, true, t) }
func TestMeshPairwise50(t *testing.T) { testMesh(50, true, t) }
