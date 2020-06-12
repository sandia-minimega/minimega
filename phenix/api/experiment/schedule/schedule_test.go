package schedule

import v1 "phenix/types/version/v1"

var nodes = []*v1.Node{
	{
		General: v1.General{
			Hostname: "foo",
		},
		Hardware: v1.Hardware{
			VCPU:   2,
			Memory: 2048,
		},
		Network: v1.Network{
			Interfaces: []v1.Interface{
				{
					VLAN: "hello",
				},
			},
		},
	},
	{
		General: v1.General{
			Hostname: "bar",
		},
		Hardware: v1.Hardware{
			VCPU:   1,
			Memory: 2048,
		},
		Network: v1.Network{
			Interfaces: []v1.Interface{
				{
					VLAN: "world",
				},
			},
		},
	},
	{
		General: v1.General{
			Hostname: "sucka",
		},
		Hardware: v1.Hardware{
			VCPU:   4,
			Memory: 8192,
		},
		Network: v1.Network{
			Interfaces: []v1.Interface{
				{
					VLAN: "hello",
				},
			},
		},
	},
	{
		General: v1.General{
			Hostname: "fish",
		},
		Hardware: v1.Hardware{
			VCPU:   1,
			Memory: 512,
		},
		Network: v1.Network{
			Interfaces: []v1.Interface{
				{
					VLAN: "world",
				},
			},
		},
	},
}
