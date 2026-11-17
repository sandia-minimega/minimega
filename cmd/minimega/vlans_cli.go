// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/vlans"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var VLANs = vlans.NewVLANs()

// maxVLANUsers limits how many VLAN users an error message names.
const maxVLANUsers = 10

// vlanUseLock lets "clear vlans blacklist" wait for the commands that looked up
// a VLAN to finish using it. Commands call Use before lookupVLAN and end the use
// after they store the VLAN in a VM, VM configuration, or host tap. Call Use
// before vmLock.
var vlanUseLock = newVLANUseTracker()

// vlanUseTracker records the commands that use VLANs in the order that they
// start to use them.
type vlanUseTracker struct {
	mu     sync.Mutex
	cond   sync.Cond
	next   uint64
	active map[uint64]bool
}

func newVLANUseTracker() *vlanUseTracker {
	t := &vlanUseTracker{active: map[uint64]bool{}}
	t.cond.L = &t.mu

	return t
}

// Use records that a command starts to use VLANs. Call the returned function
// when the command stops. Calls after the first call do nothing.
func (t *vlanUseTracker) Use() func() {
	t.mu.Lock()
	defer t.mu.Unlock()

	id := t.next
	t.next++
	t.active[id] = true

	return sync.OnceFunc(func() {
		t.mu.Lock()
		defer t.mu.Unlock()

		delete(t.active, id)
		t.cond.Broadcast()
	})
}

// Wait waits for the commands that started to use VLANs before Wait started. It
// does not wait for the commands that start later. Do not hold a lock while you
// wait.
func (t *vlanUseTracker) Wait() {
	t.mu.Lock()
	defer t.mu.Unlock()

	last := t.next
	for t.oldest() < last {
		t.cond.Wait()
	}
}

// oldest returns the ID of the oldest active use, or next if no use is active.
// The caller must hold mu.
func (t *vlanUseTracker) oldest() uint64 {
	res := t.next
	for id := range t.active {
		res = min(res, id)
	}

	return res
}

var vlansCLIHandlers = []minicli.Handler{
	{ // vlans
		HelpShort: "display allocated VLANs",
		HelpLong: `
Display information about allocated VLANs. With no arguments, prints out the
known VLAN aliases. The following subcommands are supported:

range		- view or set the VLAN range
add   		- add an alias
blacklist 	- view or create blacklisted VLAN

To remove a VLAN from the blacklist, see "clear vlans".

Note: this command is namespace aware so, for example, adding a range applies
to all *new* VLAN aliases in the current namespace.`,
		Patterns: []string{
			"vlans",
			"vlans <range,>",
			"vlans <range,> <min> <max>",
			"vlans <add,> <alias> <vlan>",
			"vlans <blacklist,> [vlan]",
		},
		Call: wrapSimpleCLI(cliVLANs),
	},
	{ // clear vlans
		HelpShort: "clear allocated VLANs",
		HelpLong: `
Clear one or more aliases, freeing the VLANs for reuse. You should only clear
allocated VLANs once you have killed all the VMs connected to them.

By default, "clear vlans" only clears aliases for the current namespace. If
given "all" as the prefix, all state about managed VLANs is cleared across
*all* namespaces, including blacklisted VLANS. You should only use this if you
want a completely clean slate.

To remove one VLAN from the blacklist on this node, use "clear vlans blacklist"
with the VLAN ID. The command fails if the VLAN is not blacklisted or in use.
The command checks the VMs on this node and on the hosts in each namespace. It
checks VM configurations, saved VM configurations, queued and launching VMs,
and host taps on this node only. The command also fails if a host in a
namespace does not return its VMs. If a host left the mesh, remove it from
each namespace with "ns del-hosts". While the command runs, other commands on
this node cannot use the VLAN. The command cannot stop commands on other hosts.
Do not use the VLAN on other hosts while the command runs. For example:

	clear vlans blacklist 100

Note: "clear vlans blacklist" without a VLAN ID clears the aliases that start
with "blacklist" in the current namespace.`,
		Patterns: []string{
			"clear vlans [prefix]",
			"clear vlans <blacklist,> <vlan>",
		},
		Call: wrapSimpleCLI(cliClearVLANs),
	},
}

var vlansCLISubHandlers = map[string]wrappedCLIFunc{
	"add":       cliVLANsAdd,
	"range":     cliVLANsRange,
	"blacklist": cliVLANsBlacklist,
}

func cliVLANs(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// Look for matching subhandler
	if len(c.BoolArgs) > 0 {
		for k, fn := range vlansCLISubHandlers {
			if c.BoolArgs[k] {
				log.Debug("vlan handler %v", k)
				return fn(ns, c, resp)
			}
		}
	}

	// No match, must want to just print
	resp.Header = []string{"alias", "vlan"}
	resp.Tabular = vlans.Tabular(ns.Name)

	return nil
}

func cliVLANsAdd(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	alias := c.StringArgs["alias"]

	vlan, err := strconv.Atoi(c.StringArgs["vlan"])
	if err != nil {
		return errors.New("expected integer VLAN")
	}

	err = vlans.AddAlias(ns.Name, alias, vlan)
	if err == nil {
		// update file so that we have a copy of the vlans if minimega crashes
		mustWrite(filepath.Join(*f_base, "vlans"), vlanInfo())

		if c.Source == "" {
			broadcastVLANAlias(ns.Name, alias, vlan)
		}
	}

	return err
}

func cliVLANsRange(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// ranges are special if we're in the default namespace -- a range gets set
	// globally for all namespaces.
	name := ns.Name
	if name == DefaultNamespace {
		name = ""
	}

	if c.StringArgs["min"] != "" && c.StringArgs["max"] != "" {
		min, err := strconv.Atoi(c.StringArgs["min"])
		max, err2 := strconv.Atoi(c.StringArgs["max"])

		if err != nil || err2 != nil {
			return errors.New("expected integer values for min/max")
		} else if max <= min {
			return errors.New("expected min > max")
		}

		return vlans.SetRange(name, min, max)
	}

	// Must want to display the ranges
	resp.Header = []string{"min", "max", "next"}
	resp.Tabular = [][]string{}

	for prefix, r := range vlans.GetRanges() {
		if name != prefix {
			continue
		}

		resp.Tabular = append(resp.Tabular,
			[]string{
				strconv.Itoa(r.Min),
				strconv.Itoa(r.Max),
				strconv.Itoa(r.Next),
			})
	}

	return nil
}

func cliVLANsBlacklist(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if v := c.StringArgs["vlan"]; v != "" {
		vlan, err := strconv.Atoi(v)
		if err != nil {
			return errors.New("expected integer VLAN")
		}

		vlans.Blacklist(vlan)
		return nil
	}

	// Must want to display the blacklisted VLANs
	resp.Header = []string{"vlan"}
	resp.Tabular = [][]string{}

	for _, v := range vlans.GetBlacklist() {
		resp.Tabular = append(resp.Tabular,
			[]string{
				strconv.Itoa(v),
			})
	}

	return nil
}

func cliClearVLANs(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["blacklist"] {
		return cliClearVLANsBlacklist(ns, c, resp)
	}

	prefix := c.StringArgs["prefix"]

	if prefix == Wildcard {
		log.Info("resetting VLAN state")
		vlans.Default = vlans.NewVLANs()
		return nil
	}

	vlans.Delete(ns.Name, prefix)
	mustWrite(filepath.Join(*f_base, "vlans"), vlanInfo())

	return nil
}

func cliClearVLANsBlacklist(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	vlan, err := strconv.Atoi(c.StringArgs["vlan"])
	if err != nil {
		return errors.New("expected integer VLAN")
	} else if vlan < 0 || vlan >= 4096 {
		return errors.New("invalid VLAN (0 <= vlan < 4096)")
	}

	// "clear vlans all" can replace vlans.Default while this command runs
	allocated := vlans.Default

	// While the VLAN is held, commands cannot look it up and start to use it
	release, err := allocated.HoldBlacklisted(vlan)
	if err != nil {
		return err
	}
	defer release()

	users, err := findVLANUsers(vlan)
	if err != nil {
		return fmt.Errorf("unable to check if VLAN %v is in use: %w", vlan, err)
	}

	if len(users) > 0 {
		slices.Sort(users)
		if len(users) > maxVLANUsers {
			users = append(users[:maxVLANUsers], fmt.Sprintf("and %v more", len(users)-maxVLANUsers))
		}

		return fmt.Errorf("VLAN %v is in use by %v", vlan, strings.Join(users, ", "))
	}

	if err := allocated.Unblacklist(vlan); err != nil {
		return err
	}

	mustWrite(filepath.Join(*f_base, "vlans"), vlanInfo())

	return nil
}

// findVLANUsers returns a description of each user of the VLAN in all the
// namespaces on this node and of each host tap on this node that uses the VLAN.
// Returns an error if a host in a namespace does not return its VMs.
func findVLANUsers(vlan int) ([]string, error) {
	nss := allNamespaces()

	var res []string

	// Wait for the commands that looked up the VLAN before the hold
	vlanUseLock.Wait()

	// Check the configurations before the VMs so that the check finds a VM
	// that launches from a configuration during the check
	for _, ns := range nss {
		res = append(res, ns.vlanConfigUsers(vlan)...)
	}

	for _, ns := range nss {
		vms, err := namespaceVMs(ns)
		if err != nil {
			return nil, fmt.Errorf("namespace %v: %w", ns.Name, err)
		}

		res = append(res, ns.vlanVMUsers(vlan, vms)...)
	}

	res = append(res, hostTapVLANUsers(vlan, bridges.HostTaps())...)

	return res, nil
}

// namespaceVMs returns the VMs in the namespace on this node and on the other
// hosts in the namespace. Unlike globalVMs, returns an error if a host does not
// return its VMs.
func namespaceVMs(ns *Namespace) ([]VM, error) {
	// This node is not always in ns.Hosts so get the local VMs directly
	resp := &minicli.Response{}
	ns.VMs.Info(nil, resp)
	vms, _ := resp.Data.([]VM)

	peers, cmds := ns.vmInfoCommands()

	peerVMs, err := collectPeerVMs(runCommands(cmds...), peers)
	if err != nil {
		return nil, err
	}

	return append(vms, peerVMs...), nil
}

// vmInfoCommands returns the hosts in the namespace other than this node and the
// commands that run `vm info` on all the hosts in the namespace.
func (n *Namespace) vmInfoCommands() ([]string, []*minicli.Command) {
	cmd := minicli.MustCompile("vm info")

	n.configLock.Lock()
	defer n.configLock.Unlock()

	// Mesh routing skips hosts without a route, so record the expected hosts
	var peers []string
	for host := range n.Hosts {
		if host != hostname {
			peers = append(peers, host)
		}
	}

	return peers, namespaceCommands(n, cmd)
}

// collectPeerVMs reads all the `vm info` responses from out and returns the
// VMs from the hosts other than this node. Returns an error for each error
// response, including the responses for hosts that time out, and for each host
// in peers that does not respond.
func collectPeerVMs(out <-chan minicli.Responses, peers []string) ([]VM, error) {
	var vms []VM
	var errs []error

	responded := map[string]bool{}

	for resps := range out {
		for _, resp := range resps {
			responded[resp.Host] = true

			// Check the error first: a failed mesh send has this node as host
			if resp.Error != "" {
				errs = append(errs, fmt.Errorf("%v: %v", resp.Host, resp.Error))
				continue
			}

			if resp.Host == hostname {
				// namespaceVMs gets the local VMs directly
				continue
			}

			switch data := resp.Data.(type) {
			case []VM:
				vms = append(vms, data...)
			case nil:
				// host has no VMs
			default:
				errs = append(errs, fmt.Errorf("%v: unknown data field in `vm info`", resp.Host))
			}
		}
	}

	slices.Sort(peers)
	for _, peer := range peers {
		if !responded[peer] {
			errs = append(errs, fmt.Errorf("%v: no response", peer))
		}
	}

	return vms, errors.Join(errs...)
}

// vlanConfigUsers returns a description of the VM configuration, each saved VM
// configuration, each queued VM batch, and each launching VM batch in the
// namespace that uses the VLAN.
//
// LOCK: The caller should call vlanUseLock.Wait first.
func (n *Namespace) vlanConfigUsers(vlan int) []string {
	n.configLock.Lock()
	defer n.configLock.Unlock()

	var res []string

	if usesVLAN(n.vmConfig.Networks, vlan) {
		res = append(res, fmt.Sprintf("vm config in namespace %v", n.Name))
	}

	for name, config := range n.savedVMConfig {
		if usesVLAN(config.Networks, vlan) {
			res = append(res, fmt.Sprintf("saved vm config %v in namespace %v", name, n.Name))
		}
	}

	for _, q := range n.queue {
		if usesVLAN(q.Networks, vlan) {
			res = append(res, fmt.Sprintf("queued VMs %v in namespace %v", strings.Join(q.Names, ","), n.Name))
		}
	}

	// The scheduler sets the names of launching VMs, so do not read them
	for q := range n.launching {
		if usesVLAN(q.Networks, vlan) {
			res = append(res, fmt.Sprintf("launching VMs in namespace %v", n.Name))
			break
		}
	}

	return res
}

// vlanVMUsers returns a description of each VM in vms that uses the VLAN.
func (n *Namespace) vlanVMUsers(vlan int, vms []VM) []string {
	var res []string

	for _, vm := range vms {
		if usesVLAN(vm.GetNetworks(), vlan) || bondsUseVLAN(vm.GetBonds(), vlan) {
			res = append(res, fmt.Sprintf("VM %v on %v in namespace %v", vm.GetName(), vm.GetHost(), n.Name))
		}
	}

	return res
}

// hostTapVLANUsers returns a description of each tap in taps that uses the
// VLAN.
func hostTapVLANUsers(vlan int, taps []bridge.Tap) []string {
	var res []string

	for _, tap := range taps {
		if tap.VLAN == vlan {
			res = append(res, fmt.Sprintf("host tap %v", tap.Name))
		}
	}

	return res
}

// usesVLAN returns true if a NIC in nics uses the VLAN.
func usesVLAN(nics []NetConfig, vlan int) bool {
	return slices.ContainsFunc(nics, func(nic NetConfig) bool {
		return nic.VLAN == vlan
	})
}

// bondsUseVLAN returns true if a created bond in bonds uses the VLAN. A bond
// keeps its VLAN when its interfaces change; until creation its VLAN is not set.
func bondsUseVLAN(bonds []BondConfig, vlan int) bool {
	return slices.ContainsFunc(bonds, func(bond BondConfig) bool {
		return bond.Created && bond.VLAN == vlan
	})
}

// cliVLANSuggest returns a list of VLAN suggestions for tab completion.
// Performs a bit of extra work to make sure that the suggestions are in the
// current namespace (completes across namespaces if prefix includes
// vlans.AliasSep).
func cliVLANSuggest(ns *Namespace, prefix string) []string {
	if !strings.Contains(prefix, vlans.AliasSep) {
		prefix = ns.Name + vlans.AliasSep + prefix
	}

	res := vlans.GetAliases(ns.Name)

	for i, v := range res {
		res[i] = strings.TrimPrefix(v, ns.Name+vlans.AliasSep)
	}

	return res
}
