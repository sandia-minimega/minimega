package main

import (
	"fmt"
	"reflect"
	"testing"
)

func testVM(vmType VMType, state VMState) VM {
	base := &BaseVM{
		Type:  vmType,
		State: state,
	}

	switch vmType {
	case KVM:
		return &KvmVM{BaseVM: base}
	case CONTAINER:
		return &ContainerVM{BaseVM: base}
	case ANDROID:
		return &AndroidVM{BaseVM: base}
	default:
		panic("unknown VM type")
	}
}

func TestVMsCountTypeStateEmpty(t *testing.T) {
	vms := &VMs{
		m: map[int]VM{},
	}

	if got := vms.CountTypeState(ANDROID, VM_KILLABLE); got != 0 {
		t.Fatalf("expected 0 Android VMs, got %d", got)
	}
}

func TestVMsCountTypeState(t *testing.T) {
	vms := &VMs{
		m: map[int]VM{
			1: testVM(ANDROID, VM_BUILDING),
			2: testVM(ANDROID, VM_RUNNING),
			3: testVM(ANDROID, VM_PAUSED),
			4: testVM(ANDROID, VM_QUIT),
			5: testVM(ANDROID, VM_ERROR),

			6: testVM(KVM, VM_RUNNING),
			7: testVM(CONTAINER, VM_RUNNING),
		},
	}

	tests := []struct {
		name   string
		vmType VMType
		mask   VMState
		want   int
	}{
		{
			name:   "active android vms",
			vmType: ANDROID,
			mask:   VM_KILLABLE,
			want:   3,
		},
		{
			name:   "quit or error android vms",
			vmType: ANDROID,
			mask:   VM_QUIT | VM_ERROR,
			want:   2,
		},
		{
			name:   "all android vms",
			vmType: ANDROID,
			mask:   VM_ANY_STATE,
			want:   5,
		},
		{
			name:   "running kvm vms",
			vmType: KVM,
			mask:   VM_RUNNING,
			want:   1,
		},
		{
			name:   "running container vms",
			vmType: CONTAINER,
			mask:   VM_RUNNING,
			want:   1,
		},
		{
			name:   "zero mask matches nothing",
			vmType: ANDROID,
			mask:   0,
			want:   0,
		},
		{
			name:   "android running excludes paused",
			vmType: ANDROID,
			mask:   VM_RUNNING,
			want:   1,
		},
		{
			name:   "kvm does not count android",
			vmType: KVM,
			mask:   VM_KILLABLE,
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vms.CountTypeState(tt.vmType, tt.mask)
			if got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

// Flush and FlushAll remove the VMs in the QUIT and ERROR states and keep
// the others. They flush the VMs in parallel, and they change the map only
// after the flushes, so the race detector finds no map access in parallel.
func TestVMsFlush(t *testing.T) {
	for _, all := range []bool{false, true} {
		name := "Flush"
		if all {
			name = "FlushAll"
		}

		t.Run(name, func(t *testing.T) {
			originalBase := *f_base
			*f_base = t.TempDir()
			t.Cleanup(func() { *f_base = originalBase })

			states := []VMState{VM_QUIT, VM_ERROR, VM_RUNNING, VM_PAUSED}
			vms := &VMs{m: map[int]VM{}}
			want := map[int]bool{}
			for i := 0; i < 16; i++ {
				state := states[i%len(states)]
				vms.m[i] = testFlushableKVM(t, i, fmt.Sprintf("vm-%d", i), "test", state)
				if state&(VM_QUIT|VM_ERROR) == 0 {
					want[i] = true
				}
			}

			var err error
			if all {
				err = vms.FlushAll(nil)
			} else {
				err = vms.Flush(Wildcard, nil)
			}
			if err != nil {
				t.Fatalf("flush: %v", err)
			}

			got := map[int]bool{}
			for id := range vms.m {
				got[id] = true
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("VMs after the flush = %v, want %v", got, want)
			}
		})
	}
}
