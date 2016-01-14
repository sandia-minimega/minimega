// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"gopacket/macs"
	_ "gopnm"
	"image"
	"image/png"
	"io/ioutil"
	"math/rand"
	"minicli"
	log "minilog"
	"net"
	"os/exec"
	"regexp"
	"resize"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type errSlice []error

var validMACPrefix [][3]byte

func init() {
	for k, _ := range macs.ValidMACPrefixMap {
		validMACPrefix = append(validMACPrefix, k)
	}
}

func (errs errSlice) String() string {
	vals := []string{}
	for _, err := range errs {
		vals = append(vals, err.Error())
	}
	return strings.Join(vals, "\n")
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

func isMac(mac string) bool {
	match, err := regexp.MatchString("^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$", mac)
	if err != nil {
		return false
	}
	return match
}

func allocatedMac(mac string) bool {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return false
	}

	_, allocated := macs.ValidMACPrefixMap[[3]byte{hw[0], hw[1], hw[2]}]
	return allocated
}

func hostid(s string) (string, int) {
	k := strings.Split(s, ":")
	if len(k) != 2 {
		log.Error("hostid cannot split host vmid pair: %v", k)
		return "", -1
	}
	val, err := strconv.Atoi(k[1])
	if err != nil {
		log.Error("parse hostid: %v", err)
		return "", -1
	}
	return k[0], val
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

// cmdTimeout runs the command c and returns a timeout if it doesn't complete
// after time t. If a timeout occurs, cmdTimeout will kill the process.
func cmdTimeout(c *exec.Cmd, t time.Duration) error {
	log.Debug("cmdTimeout: %v", c)

	start := time.Now()
	err := c.Start()
	if err != nil {
		return fmt.Errorf("cmd start: %v", err)
	}

	done := make(chan error)
	go func() {
		done <- c.Wait()
	}()

	select {
	case <-time.After(t):
		err = c.Process.Kill()
		if err != nil {
			return err
		}
		return <-done
	case err = <-done:
		log.Debug("cmd %v completed in %v", c, time.Now().Sub(start))
		return err
	}
}

// findRemoteVM attempts to find the VM ID of a VM by name or ID on a remote
// minimega node. It returns the ID of the VM on the remote host or an error,
// which may also be an error communicating with the remote node.
func findRemoteVM(host, vm string) (int, string, error) {
	log.Debug("findRemoteVM: %v %v", host, vm)

	// check for our own host
	if host == hostname || host == Localhost {
		log.Debugln("host is local node")
		vm := vms.findVm(vm)
		if vm != nil {
			log.Debug("got vm: %v %v %v", host, vm.GetID, vm.GetName())
			return vm.GetID(), vm.GetName(), nil
		}
	} else {
		log.Debugln("remote host")

		var cmdStr string
		v, err := strconv.Atoi(vm)
		if err == nil {
			cmdStr = fmt.Sprintf(".filter id=%v .columns name,id .record false vm info", v)
		} else {
			cmdStr = fmt.Sprintf(".filter name=%v .columns name,id .record false vm info", vm)
		}

		cmd := minicli.MustCompile(cmdStr)

		remoteRespChan := make(chan minicli.Responses)
		go func() {
			meshageSend(cmd, host, remoteRespChan)
			close(remoteRespChan)
		}()

		for resps := range remoteRespChan {
			// Find a response that is not an error
			for _, resp := range resps {
				if resp.Error == "" && len(resp.Tabular) > 0 {
					// Found it!
					row := resp.Tabular[0] // should be name,id
					name := row[0]
					id, err := strconv.Atoi(row[1])
					if err != nil {
						log.Debug("malformed response: %#v", resp)
					} else {
						return id, name, nil
					}
				}
			}
		}
	}

	return 0, "", vmNotFound(vm)
}

// registerHandlers registers all the provided handlers with minicli, panicking
// if any of the handlers fail to register.
func registerHandlers(name string, handlers []minicli.Handler) {
	for i := range handlers {
		err := minicli.Register(&handlers[i])
		if err != nil {
			log.Fatal("invalid handler, %s:%d -- %v", name, i, err)
		}
	}
}

// makeIDChan creates a channel of IDs and a goroutine to populate the channel
// with a counter. This is useful for assigning UIDs to fields since the
// goroutine will (almost) never repeat the same value (unless we hit IntMax).
func makeIDChan() chan int {
	idChan := make(chan int)

	go func() {
		for i := 0; ; i++ {
			idChan <- i
		}
	}()

	return idChan
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
