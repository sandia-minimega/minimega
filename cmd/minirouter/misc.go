package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ARPHRD_ETHER, the /sys/class/net/<iface>/type of an Ethernet interface
const arphrdEther = "1"

var (
	sysClassNet = "/sys/class/net"

	// fallbackTunnels are created by the kernel in every network namespace
	// once their module is loaded (for example, by a GRE tunnel on the host),
	// so they are never interfaces minimega gave the router (issue #1387).
	fallbackTunnels = map[string]bool{
		"erspan0":  true,
		"gre0":     true,
		"gretap0":  true,
		"ip6_vti0": true,
		"ip6gre0":  true,
		"ip6tnl0":  true,
		"ip_vti0":  true,
		"sit0":     true,
		"tunl0":    true,
	}
)

// findEth returns the name of the router's idx'th network interface.
func findEth(idx int) (string, error) {
	ethNames, err := listEths(sysClassNet)
	if err != nil {
		return "", err
	}
	if idx >= 0 && idx < len(ethNames) {
		return ethNames[idx], nil
	}
	return "", fmt.Errorf("no such network")
}

// listEths returns the Ethernet interfaces in dir (a /sys/class/net), in the
// order minimega assigned them: by the integer suffix of their names.
func listEths(dir string) ([]string, error) {
	dirs, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, n := range dirs {
		name := n.Name()
		if name == "lo" || fallbackTunnels[name] {
			continue
		}
		// skip non-Ethernet devices such as tunnels; keep the interface if
		// its type cannot be read
		if t, err := os.ReadFile(filepath.Join(dir, name, "type")); err == nil && strings.TrimSpace(string(t)) != arphrdEther {
			continue
		}
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool {
		return ethLess(names[i], names[j])
	})
	return names, nil
}

// ethLess orders interface names by their integer suffix (eth2 before eth10),
// then by name so the order is deterministic.
func ethLess(a, b string) bool {
	idxA, okA := ethIndex(a)
	idxB, okB := ethIndex(b)
	switch {
	case okA && okB && idxA != idxB:
		return idxA < idxB
	case okA != okB:
		return okA
	}
	return a < b
}

// ethIndex returns the integer suffix of an interface name such as eth0.
func ethIndex(name string) (int, bool) {
	prefix := strings.TrimRight(name, "0123456789")
	idx, err := strconv.Atoi(name[len(prefix):])
	return idx, err == nil
}

func isIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

func isIPv6(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil
}
