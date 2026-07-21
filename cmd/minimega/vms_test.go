package main

import "testing"

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
