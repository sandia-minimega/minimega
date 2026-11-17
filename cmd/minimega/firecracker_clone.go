// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	// fcCloneTag is the tag that shows the identity state of a clone. It is
	// also the prefix of the cc commands of the identity.
	fcCloneTag = "clone_identity"

	// fcCloneDHCPMarker is the line that the identity script prints when
	// udhcpc runs in the guest.
	fcCloneDHCPMarker = "minimega-clone-dhcp"

	// fcCloneHostnameMax is the size limit of the guest host name, the limit
	// of a DNS label.
	fcCloneHostnameMax = 63

	// fcCloneNICFunc is a shell function that prints the name of the guest
	// NIC with MAC $1. It skips links without a device, for example bridges.
	fcCloneNICFunc = `mm_nic() {
	for mm_a in /sys/class/net/*/address; do
		mm_d=${mm_a%/address}
		if [ -e "$mm_d/device" ] && [ "$(cat "$mm_a")" = "$1" ]; then
			echo "${mm_d##*/}"
			return 0
		fi
	done
	echo "no NIC has MAC $1" >&2
	return 1
}
`
)

// Tests replace these.
var (
	// fcCloneTimeout is how long a clone waits for its cc client, and then
	// for the response to each cc command. The time while the VM is paused
	// does not count. fcClonePoll is the check interval.
	fcCloneTimeout = 120 * time.Second
	fcClonePoll    = 250 * time.Millisecond

	// fcCloneConnect moves NIC i of vm to vlan on bridge br.
	//
	// LOCK: Assumes vm.lock is held.
	fcCloneConnect = func(vm *FirecrackerVM, i, vlan int, br string) error {
		return vm.networkConnect(i, vlan, br)
	}
)

// fcCloneCC is the part of the ron server that the identity of a clone uses.
type fcCloneCC interface {
	HasClient(string) bool
	NewCommand(*ron.Command) int
	GetCommand(int) *ron.Command
	GetExitCode(int, string) (int, error)
	GetResponse(int, bool) (string, error)
	DeleteCommand(int) error
	DeleteResponse(int) error
}

var _ fcCloneCC = (*ron.Server)(nil)

// fcClone is the identity that a restored clone waits for.
type fcClone struct {
	from  []string // the MAC of each NIC in the snapshot
	vlans []int    // the VLAN of each NIC after the identity
}

func (c *fcClone) copy() *fcClone {
	return &fcClone{from: slices.Clone(c.from), vlans: slices.Clone(c.vlans)}
}

// holdClone records the VLAN of each NIC and puts the NICs on
// DisconnectedVLAN, so that the taps of a clone get no bridge port before its
// identity is set. from holds the NIC MACs of the snapshot.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) holdClone(from []string) {
	if vm.clone != nil {
		return
	}

	c := &fcClone{}
	for i := range vm.Networks {
		nic := &vm.Networks[i]

		mac := ""
		if i < len(from) {
			mac = strings.ToLower(from[i])
		}
		c.from = append(c.from, mac)

		if nic.Tap != "" {
			c.vlans = append(c.vlans, DisconnectedVLAN)
			continue
		}
		c.vlans = append(c.vlans, nic.VLAN)
		nic.VLAN = DisconnectedVLAN
		nic.Alias = ""
	}

	vm.clone = c
}

// startCloneIdentity starts the identity of a clone that vm start resumed.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) startCloneIdentity() {
	c := vm.clone
	if c == nil {
		return
	}

	var macs []string
	for _, nic := range vm.Networks {
		macs = append(macs, strings.ToLower(nic.MAC))
	}

	vm.Tags[fcCloneTag] = "pending"
	log.Info("vm %v: the clone waits for its cc client to set its identity", vm.ID)

	go vm.cloneIdentity(vm.gen, vm.UUID, fcCloneScript(c.from, macs, vm.Name), macs)
}

// cloneIdentity sets the identity of the clone of launch gen through cc, then
// connects its NICs. It does not hold vm.lock during the cc calls.
func (vm *FirecrackerVM) cloneIdentity(gen int, uuid, script string, macs []string) {
	alive := func() (bool, error) {
		_, paused, err := vm.cloneCC(gen)
		return paused, err
	}

	s, err := vm.waitCloneClient(gen, uuid)

	var out string
	if err == nil {
		out, err = fcCloneRun(s, uuid, script, alive)
	}

	if err := vm.finishClone(gen, err); err != nil {
		return
	}

	if !slices.Contains(strings.Split(out, "\n"), fcCloneDHCPMarker) {
		return
	}
	if _, err := fcCloneRun(s, uuid, fcCloneDHCPScript(macs), alive); err != nil {
		log.Warn("vm %v: udhcpc in the clone: %v", vm.ID, err)
	}
}

// cloneCC returns the cc server of the VM, and true when the VM is paused. It
// returns an error when the VM stopped, or when a relaunch or a flush replaced
// launch gen.
func (vm *FirecrackerVM) cloneCC(gen int) (fcCCServer, bool, error) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.gen != gen {
		return nil, false, errors.New("the VM was relaunched or flushed before its identity was set")
	}
	if vm.State == VM_QUIT || vm.State == VM_ERROR {
		return nil, false, fmt.Errorf("the VM is %v", vm.State)
	}

	return vm.cc, vm.State == VM_PAUSED, nil
}

// fcCloneClock counts the time of a clone wait while the VM is not paused.
type fcCloneClock struct {
	left time.Duration
	last time.Time
}

func newFCCloneClock() *fcCloneClock {
	return &fcCloneClock{left: fcCloneTimeout, last: time.Now()}
}

// expired adds the time since the last call when the VM was not paused, and
// returns true when the wait used all of fcCloneTimeout.
func (c *fcCloneClock) expired(paused bool) bool {
	now := time.Now()
	if !paused {
		c.left -= now.Sub(c.last)
	}
	c.last = now

	return c.left <= 0
}

// waitCloneClient waits until the cc server of the VM has a client with UUID
// uuid, for at most fcCloneTimeout.
func (vm *FirecrackerVM) waitCloneClient(gen int, uuid string) (fcCloneCC, error) {
	clock := newFCCloneClock()

	for {
		cc, paused, err := vm.cloneCC(gen)
		if err != nil {
			return nil, err
		}

		if cc != nil {
			s, ok := cc.(fcCloneCC)
			if !ok {
				return nil, errors.New("the cc server cannot run commands")
			}
			if s.HasClient(uuid) {
				return s, nil
			}
		}

		if clock.expired(paused) {
			return nil, fmt.Errorf("no cc client connected in %v", fcCloneTimeout)
		}
		time.Sleep(fcClonePoll)
	}
}

// fcCloneRun runs script with sh in the guest of cc client uuid, as one cc
// command, and waits for the response for at most fcCloneTimeout. It returns
// the output, and an error when the script fails or alive returns an error.
// alive also returns true when the VM is paused. fcCloneRun then deletes the
// command and its response.
func fcCloneRun(s fcCloneCC, uuid, script string, alive func() (bool, error)) (string, error) {
	id := s.NewCommand(&ron.Command{
		Command: []string{"sh", "-c", script},
		Filter:  &ron.Filter{UUID: uuid},
		Prefix:  fcCloneTag,
	})
	defer func() {
		if err := s.DeleteCommand(id); err != nil {
			log.Debug("delete cc command %v: %v", id, err)
		}
		if err := s.DeleteResponse(id); err != nil {
			log.Debug("delete cc response %v: %v", id, err)
		}
	}()

	clock := newFCCloneClock()

	for {
		c := s.GetCommand(id)
		if c == nil {
			return "", fmt.Errorf("cc command %v was deleted", id)
		}

		// The exit code arrives after the check-in.
		if slices.Contains(c.CheckedIn, uuid) {
			if code, err := s.GetExitCode(id, uuid); err == nil {
				out, _ := s.GetResponse(id, true)
				out = strings.TrimSpace(out)
				if code == 0 {
					return out, nil
				}

				msg := fmt.Sprintf("cc command exited with status %d", code)
				if out != "" {
					msg += ": " + strings.Join(strings.Fields(out), " ")
				}
				return out, errors.New(msg)
			}
		}

		paused, err := alive()
		if err != nil {
			return "", err
		}
		if clock.expired(paused) {
			return "", fmt.Errorf("no response to cc command %v in %v", id, fcCloneTimeout)
		}
		time.Sleep(fcClonePoll)
	}
}

// finishClone ends the identity of the clone of launch gen. When err is nil,
// it connects each NIC that is still disconnected to its VLAN. When the guest
// of launch gen is gone, it also gives the NICs their VLANs. When the guest
// runs without its identity, the NICs stay disconnected, and vm.clone stays
// set. It records the result in the clone_identity tag, and returns the error.
func (vm *FirecrackerVM) finishClone(gen int, err error) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	c := vm.clone
	vm.clone = nil

	if err == nil && (c == nil || vm.gen != gen) {
		err = errors.New("the VM was relaunched or flushed before its identity was set")
	}

	if err == nil {
		err = vm.connectClone(c)
	}

	if err != nil {
		vm.Tags[fcCloneTag] = err.Error()
		log.Error("vm %v: clone identity: %v", vm.ID, err)

		if c != nil && vm.gen == gen && vm.State&(VM_QUIT|VM_ERROR) == 0 {
			vm.clone = c
		} else if c != nil {
			vm.releaseClone(c)
		}
		return err
	}

	vm.Tags[fcCloneTag] = "ok"
	log.Info("vm %v: clone identity is set", vm.ID)

	return nil
}

// connectClone connects each NIC of clone c that is still disconnected to its
// VLAN.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) connectClone(c *fcClone) error {
	var errs []string
	for i, vlan := range c.vlans {
		if vlan == DisconnectedVLAN || i >= len(vm.Networks) || vm.Networks[i].VLAN != DisconnectedVLAN {
			continue
		}
		if err := fcCloneConnect(vm, i, vlan, vm.Networks[i].Bridge); err != nil {
			errs = append(errs, fmt.Sprintf("connect NIC %d: %v", i, err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

// releaseClone gives the NICs of clone c the VLANs that holdClone recorded,
// when the guest that waited for the identity is gone. A NIC without a tap
// gets its VLAN in the configuration only.
//
// LOCK: Assumes vm.lock is held.
func (vm *FirecrackerVM) releaseClone(c *fcClone) {
	for i, vlan := range c.vlans {
		if vlan == DisconnectedVLAN || i >= len(vm.Networks) || vm.Networks[i].VLAN != DisconnectedVLAN {
			continue
		}

		nic := &vm.Networks[i]
		if nic.Tap == "" || vm.netns == nil {
			nic.VLAN = vlan
			continue
		}
		if err := fcCloneConnect(vm, i, vlan, nic.Bridge); err != nil {
			log.Error("vm %v: connect NIC %d of the clone: %v", vm.ID, i, err)
		}
	}
}

// fcCloneScript returns the identity script of a clone. For each NIC, it
// finds the guest NIC with MAC from[i] and gives it MAC to[i]. A live change
// keeps the addresses and routes of the NIC. When the driver refuses a live
// change, the script sets the NIC down for the change. Then the script sets
// the host name to the first fcCloneHostnameMax bytes of name, and prints
// fcCloneDHCPMarker when udhcpc runs in the guest. A failure to set the host
// name does not stop the script.
func fcCloneScript(from, to []string, name string) string {
	var b strings.Builder

	b.WriteString("set -e\n")
	b.WriteString(fcCloneNICFunc)
	for i := range to {
		fmt.Fprintf(&b, "d%d=$(mm_nic %v)\n", i, from[i])
	}
	for i, mac := range to {
		d := fmt.Sprintf(`"$d%d"`, i)
		fmt.Fprintf(&b, "ip link set dev %v address %v 2>/dev/null || { ip link set dev %v down && ip link set dev %v address %v && ip link set dev %v up; }\n", d, mac, d, d, mac, d)
	}
	fmt.Fprintf(&b, "hostname -- %v || true\n", fcShellQuote(fcCloneHostname(name)))
	fmt.Fprintf(&b, "if pidof udhcpc >/dev/null 2>&1; then echo %v; fi\n", fcCloneDHCPMarker)

	return b.String()
}

// fcCloneHostname returns the first fcCloneHostnameMax bytes of name, without
// a part of a UTF-8 character at the end.
func fcCloneHostname(name string) string {
	if len(name) <= fcCloneHostnameMax {
		return name
	}

	name = name[:fcCloneHostnameMax]
	for len(name) > 0 && !utf8.ValidString(name) {
		name = name[:len(name)-1]
	}

	return name
}

// fcCloneDHCPScript returns the script that runs udhcpc one time on the guest
// NIC with each MAC in macs, to get a lease for the MAC of the clone.
func fcCloneDHCPScript(macs []string) string {
	var b strings.Builder

	b.WriteString("r=0\n")
	b.WriteString(fcCloneNICFunc)
	for _, mac := range macs {
		fmt.Fprintf(&b, "d=$(mm_nic %v) && udhcpc -i \"$d\" -n -q -t 3 || r=1\n", mac)
	}
	b.WriteString("exit $r\n")

	return b.String()
}

// fcShellQuote quotes s for sh.
func fcShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
