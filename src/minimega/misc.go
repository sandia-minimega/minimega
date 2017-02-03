// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"math/rand"
	"minicli"
	log "minilog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"vlans"

	"github.com/google/gopacket/macs"
	_ "github.com/jbuchbinder/gopnm"
	"github.com/nfnt/resize"
)

type errSlice []error

// loggingMutex logs whenever it is locked or unlocked with the file and line
// number of the caller. Can be swapped for sync.Mutex to track down deadlocks.
type loggingMutex struct {
	sync.Mutex // embed
}

var validMACPrefix [][3]byte

func init() {
	for k, _ := range macs.ValidMACPrefixMap {
		validMACPrefix = append(validMACPrefix, k)
	}
}

// makeErrSlice turns a slice of errors into an errSlice which implements the
// Error interface. This checks to make sure that there is at least one non-nil
// error in the slice and returns nil otherwise.
func makeErrSlice(errs []error) error {
	var found bool

	for _, err := range errs {
		if err != nil {
			found = true
			break
		}
	}

	if !found {
		return nil
	}

	return errSlice(errs)
}

func (errs errSlice) Error() string {
	return errs.String()
}

func (errs errSlice) String() string {
	vals := []string{}
	for _, err := range errs {
		if err != nil {
			vals = append(vals, err.Error())
		}
	}
	return strings.Join(vals, "\n")
}

func (m *loggingMutex) Lock() {
	_, file, line, _ := runtime.Caller(1)

	log.Info("locking: %v:%v", file, line)
	m.Mutex.Lock()
	log.Info("locked: %v:%v", file, line)
}

func (m *loggingMutex) Unlock() {
	_, file, line, _ := runtime.Caller(1)

	log.Info("unlocking: %v:%v", file, line)
	m.Mutex.Unlock()
	log.Info("unlocked: %v:%v", file, line)
}

func generateUUID() string {
	log.Debugln("generateUUID")
	uuid, err := ioutil.ReadFile("/proc/sys/kernel/random/uuid")
	if err != nil {
		log.Error("generateUUID: %v", err)
		return "00000000-0000-0000-0000-000000000000"
	}
	uuid = uuid[:len(uuid)-1]
	log.Debug("generated UUID: %v", string(uuid))
	return string(uuid)
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}

	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return false
	}

	for i, v := range []int{8, 4, 4, 4, 12} {
		if len(parts[i]) != v {
			return false
		}

		if _, err := strconv.ParseInt(parts[i], 16, 64); err != nil {
			return false
		}
	}

	return true
}

// generate a random mac address and return as a string
func randomMac() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	//
	prefix := validMACPrefix[r.Intn(len(validMACPrefix))]

	mac := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", prefix[0], prefix[1], prefix[2], r.Intn(256), r.Intn(256), r.Intn(256))
	log.Info("generated mac: %v", mac)
	return mac
}

func isMac(mac string) bool {
	_, err := net.ParseMAC(mac)
	return err == nil
}

func allocatedMac(mac string) bool {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return false
	}

	_, allocated := macs.ValidMACPrefixMap[[3]byte{hw[0], hw[1], hw[2]}]
	return allocated
}

// Return a slice of strings, split on whitespace, not unlike strings.Fields(),
// except that quoted fields are grouped.
// 	Example: a b "c d"
// 	will return: ["a", "b", "c d"]
func fieldsQuoteEscape(c string, input string) []string {
	log.Debug("fieldsQuoteEscape splitting on %v: %v", c, input)
	f := strings.Fields(input)
	var ret []string
	trace := false
	temp := ""

	for _, v := range f {
		if trace {
			if strings.Contains(v, c) {
				trace = false
				temp += " " + trimQuote(c, v)
				ret = append(ret, temp)
			} else {
				temp += " " + v
			}
		} else if strings.Contains(v, c) {
			temp = trimQuote(c, v)
			if strings.HasSuffix(v, c) {
				// special case, single word like 'foo'
				ret = append(ret, temp)
			} else {
				trace = true
			}
		} else {
			ret = append(ret, v)
		}
	}
	log.Debug("generated: %#v", ret)
	return ret
}

func trimQuote(c string, input string) string {
	if c == "" {
		log.Errorln("cannot trim empty space")
		return ""
	}
	var ret string
	for _, v := range input {
		if v != rune(c[0]) {
			ret += string(v)
		}
	}
	return ret
}

func unescapeString(input []string) string {
	var ret string
	for _, v := range input {
		containsWhite := false
		for _, x := range v {
			if unicode.IsSpace(x) {
				containsWhite = true
				break
			}
		}
		if containsWhite {
			ret += fmt.Sprintf(" \"%v\"", v)
		} else {
			ret += fmt.Sprintf(" %v", v)
		}
	}
	log.Debug("unescapeString generated: %v", ret)
	return strings.TrimSpace(ret)
}

// convert a src ppm image to a dst png image, resizing to a largest dimension
// max if max != 0
func ppmToPng(src []byte, max int) ([]byte, error) {
	in := bytes.NewReader(src)

	img, _, err := image.Decode(in)
	if err != nil {
		return nil, err
	}

	// resize the image if necessary
	if max != 0 {
		img = resize.Thumbnail(uint(max), uint(max), img, resize.NearestNeighbor)
	}

	out := new(bytes.Buffer)

	err = png.Encode(out, img)
	if err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// hasCommand tests whether cmd or any of it's subcommand has the given prefix.
// This is used to ensure that certain commands don't get nested such as `read`
// and `mesh send`.
func hasCommand(cmd *minicli.Command, prefix string) bool {
	return strings.HasPrefix(cmd.Original, prefix) ||
		(cmd.Subcommand != nil && hasCommand(cmd.Subcommand, prefix))
}

// isReserved checks whether the provided string is a reserved identifier.
func isReserved(s string) bool {
	for _, r := range reserved {
		if r == s {
			return true
		}
	}

	return false
}

// hasWildcard tests whether the lookup table has Wildcard set. If it does, and
// there are more keys set than just the Wildcard, it logs a message.
func hasWildcard(v map[string]bool) bool {
	if v[Wildcard] && len(v) > 1 {
		log.Info("found wildcard amongst names, making command wild")
	}

	return v[Wildcard]
}

// writeOrDie writes data to the provided file. If there is an error, calls
// teardown to kill minimega.
func writeOrDie(fpath, data string) {
	if err := ioutil.WriteFile(fpath, []byte(data), 0664); err != nil {
		log.Errorln(err)
		teardown()
	}
}

// PermStrings creates a random permutation of the source slice using the
// "inside-out" version of the Fisher-Yates algorithm.
func PermStrings(source []string) []string {
	res := make([]string, len(source))

	for i := range source {
		j := rand.Intn(i + 1)
		if j != i {
			res[i] = res[j]
		}
		res[j] = source[i]
	}

	return res
}

// processVMNet processes the input specifying the bridge, vlan, and mac for
// one interface to a VM and updates the vm config accordingly. This takes a
// bit of parsing, because the entry can be in a few forms:
// 	vlan
//
//	vlan,mac
//	bridge,vlan
//	vlan,driver
//
//	bridge,vlan,mac
//	vlan,mac,driver
//	bridge,vlan,driver
//
//	bridge,vlan,mac,driver
// If there are 2 or 3 fields, just the last field for the presence of a mac
func processVMNet(spec string) (res NetConfig, err error) {
	// example: my_bridge,100,00:00:00:00:00:00
	f := strings.Split(spec, ",")

	var b, v, m, d string
	switch len(f) {
	case 1:
		v = f[0]
	case 2:
		if isMac(f[1]) {
			// vlan, mac
			v, m = f[0], f[1]
		} else if isNetworkDriver(f[1]) {
			// vlan, driver
			v, d = f[0], f[1]
		} else {
			// bridge, vlan
			b, v = f[0], f[1]
		}
	case 3:
		if isMac(f[2]) {
			// bridge, vlan, mac
			b, v, m = f[0], f[1], f[2]
		} else if isMac(f[1]) {
			// vlan, mac, driver
			v, m, d = f[0], f[1], f[2]
		} else {
			// bridge, vlan, driver
			b, v, d = f[0], f[1], f[2]
		}
	case 4:
		b, v, m, d = f[0], f[1], f[2], f[3]
	default:
		return NetConfig{}, errors.New("malformed netspec")
	}

	if d != "" && !isNetworkDriver(d) {
		return NetConfig{}, errors.New("malformed netspec, invalid driver: " + d)
	}

	log.Debug("got bridge=%v, vlan=%v, mac=%v, driver=%v", b, v, m, d)

	vlan, err := lookupVLAN(v)
	if err != nil {
		return NetConfig{}, err
	}

	if m != "" && !isMac(m) {
		return NetConfig{}, errors.New("malformed netspec, invalid mac address: " + m)
	}

	// warn on valid but not allocated macs
	if m != "" && !allocatedMac(m) {
		log.Warn("unallocated mac address: %v", m)
	}

	if b == "" {
		b = DefaultBridge
	}
	if d == "" {
		d = VM_NET_DRIVER_DEFAULT
	}

	return NetConfig{
		VLAN:   vlan,
		Bridge: b,
		MAC:    strings.ToLower(m),
		Driver: d,
	}, nil
}

// lookupVLAN uses the allocatedVLANs and active namespace to turn a string
// into a VLAN. If the VLAN didn't already exist, broadcasts the update to the
// cluster.
func lookupVLAN(alias string) (int, error) {
	namespace := GetNamespaceName()

	vlan, err := allocatedVLANs.ParseVLAN(namespace, alias)
	if err != vlans.ErrUnallocated {
		// nil or other error
		return vlan, err
	}

	vlan, created, err := allocatedVLANs.Allocate(namespace, alias)
	if err != nil {
		return 0, err
	}

	if !created {
		return vlan, nil
	}

	// Broadcast out vlan alias to everyone so that we have a record of the
	// aliases, should this node crash.
	s := fmt.Sprintf("vlans add %q %v", alias, vlan)
	if namespace != "" && !strings.Contains(alias, vlans.AliasSep) {
		s = fmt.Sprintf("namespace %q %v", namespace, s)
	}
	cmd := minicli.MustCompile(s)
	cmd.SetRecord(false)
	cmd.SetSource(namespace)

	respChan, err := meshageSend(cmd, Wildcard)
	if err != nil {
		// don't propagate the error since this is supposed to be best-effort.
		log.Error("unable to broadcast alias update: %v", err)
		return vlan, nil
	}

	// read all the responses, looking for errors
	go func() {
		for resps := range respChan {
			for _, resp := range resps {
				if resp.Error != "" {
					log.Info("unable to send alias %v -> %v to %v: %v", alias, vlan, resp.Host, resp.Error)
				}
			}
		}

	}()

	return vlan, nil
}

// printVLAN uses the allocatedVLANs and active namespace to print a vlan.
func printVLAN(vlan int) string {
	namespace := GetNamespaceName()

	return allocatedVLANs.PrintVLAN(namespace, vlan)
}

// wget downloads a URL and writes it to disk, creates parent directories if
// needed.
func wget(u, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
