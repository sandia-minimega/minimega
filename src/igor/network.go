package main

var (
	networkSetFuncs   map[string]func([]string, int) error
	networkClearFuncs map[string]func([]string) error
)

func networkSet(nodes []string, vlan int) error {
	f, ok := networkSetFuncs[igorConfig.Network]
	if !ok {
		fatalf("no such network mode: %v", igorConfig.Network)
	}
	return f(nodes, vlan)
}

func networkClear(nodes []string) error {
	f, ok := networkClearFuncs[igorConfig.Network]
	if !ok {
		fatalf("no such network mode: %v", igorConfig.Network)
	}
	return f(nodes)
}
