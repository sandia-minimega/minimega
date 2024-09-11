// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/sandia-minimega/minimega/v2/internal/vlans"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	_ "github.com/jbuchbinder/gopnm"
	"github.com/nfnt/resize"
)

type errSlice []error

// loggingMutex logs whenever it is locked or unlocked with the file and line
// number of the caller. Can be swapped for sync.Mutex to track down deadlocks.
type loggingMutex struct {
	sync.Mutex // embed
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

// unreachable returns an error when we reach a condition that should be
// unreachable and tags the file/line number. This usually means our CLI
// handling is wrong (i.e. we missed a case).
func unreachable() error {
	_, file, line, _ := runtime.Caller(1)

	return fmt.Errorf("unreachable %v:%v, please report.", file, line)
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

// generate a random mac address and return as a string
func randomMac() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	//
	prefix := validMACPrefix[r.Intn(len(validMACPrefix))]

	mac := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", prefix[0], prefix[1], prefix[2], r.Intn(256), r.Intn(256), r.Intn(256))
	log.Info("generated mac: %v", mac)
	return mac
}

// Return a slice of strings, split on whitespace, not unlike strings.Fields(),
// except that quoted fields are grouped.
//
//	Example: a b "c d"
//	will return: ["a", "b", "c d"]
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

// quoteJoin joins elements from s with sep, quoting any element containing a
// space.
func quoteJoin(s []string, sep string) string {
	s2 := make([]string, len(s))

	for i := range s {
		if strings.IndexFunc(s[i], unicode.IsSpace) > -1 {
			s2[i] = strconv.Quote(s[i])
		} else {
			s2[i] = s[i]
		}
	}

	return strings.Join(s2, sep)
}

func humanReadableBytes(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
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

// mustWrite writes data to the provided file. If there is an error, calls
// log.Fatal to kill minimega.
func mustWrite(fpath, data string) {
	log.Debug("writing to %v", fpath)

	if err := ioutil.WriteFile(fpath, []byte(data), 0664); err != nil {
		log.Fatal("write %v failed: %v", fpath, err)
	}
}

// marshal returns the JSON-marshaled version of `v`. If we are unable to
// marshal it for whatever reason, we log an error and return an empty string.
func marshal(v interface{}) string {
	if v == nil {
		return ""
	}

	b, err := json.Marshal(v)
	if err != nil {
		log.Error("unable to marshal %v: %v", v, err)
		return ""
	}

	return string(b)
}

func checkPath(v string) string {
	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(v) {
		v = filepath.Join(*f_iomBase, v)
	}

	if _, err := os.Stat(v); os.IsNotExist(err) {
		log.Warn("file does not exist: %v", v)
	}

	return v
}

// lookupVLAN uses the vlans and active namespace to turn a string into a VLAN.
// If the VLAN didn't already exist, broadcasts the update to the cluster.
func lookupVLAN(namespace, alias string) (int, error) {
	if alias == "" {
		return 0, errors.New("VLAN must be non-empty string")
	}

	vlan, err := vlans.ParseVLAN(namespace, alias)
	if err != vlans.ErrUnallocated {
		// nil or other error
		return vlan, err
	}

	vlan, created, err := vlans.Allocate(namespace, alias)
	if err != nil {
		return 0, err
	}

	if created {
		// update file so that we have a copy of the vlans if minimega crashes
		mustWrite(filepath.Join(*f_base, "vlans"), vlanInfo())

		// broadcast out the alias to the cluster so that the other nodes can
		// print the alias correctly
		cmd := minicli.MustCompilef("namespace %v vlans add %q %v", namespace, alias, vlan)
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
						log.Error("unable to send alias %v -> %v to %v: %v", alias, vlan, resp.Host, resp.Error)
					}
				}
			}
		}()
	}

	return vlan, nil
}

func recoverVLANs() error {
	f, err := os.Open(filepath.Join(*f_base, "vlans"))
	if err == nil {
		var (
			scanner = bufio.NewScanner(f)
			skip    = true
		)

		for scanner.Scan() {
			if skip {
				// skip first line in file (header data)
				skip = false
				continue
			}

			fields := strings.Fields(scanner.Text())

			if len(fields) != 2 {
				return fmt.Errorf("expected exactly two columns in vlans file: got %d", len(fields))
			}

			alias := fields[0]
			vlan, err := strconv.Atoi(fields[1])
			if err != nil {
				return fmt.Errorf("invalid VLAN ID %s for alias %s provided in vlans file: %w", fields[1], alias, err)
			}

			if err := vlans.AddAlias("", alias, vlan); err != nil {
				return fmt.Errorf("unable to add VLAN alias %s (ID %d): %w", alias, vlan, err)
			}
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("unable to process vlans file: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("unable to open vlans file: %w", err)
	}

	return nil
}

// printVLAN uses the vlans and active namespace to print a vlan.
func printVLAN(namespace string, vlan int) string {
	return vlans.PrintVLAN(namespace, vlan)
}

// vlanInfo returns formatted information about all the vlans.
func vlanInfo() string {
	info := vlans.Tabular("")
	if len(info) == 0 {
		return ""
	}

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "Alias\tVLAN\n")
	for _, i := range info {
		fmt.Fprintf(w, "%v\t%v\n", i[0], i[1])
	}

	w.Flush()
	return o.String()
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

func writeInt(filename string, value int) error {
	log.Debug("writing %v to %v", value, filename)

	b := []byte(strconv.Itoa(value))
	return ioutil.WriteFile(filename, b, 0644)
}

func readInt(filename string) (int, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return 0, fmt.Errorf("unable to read %v: %v", filename, err)
	}

	s := strings.TrimSpace(string(b))

	run, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("expected int from %v, not `%v`", filename, s)
	}

	log.Debug("got %v from %v", int(run), filename)

	return int(run), nil
}

func mesh(vals []string, pairwise bool) map[string][]string {
	res := make(map[string][]string)

	if pairwise {
		for _, v := range vals {
			for _, v2 := range vals {
				if v == v2 {
					continue
				}

				res[v] = append(res[v], v2)
			}
		}

		return res
	}

	n := uint(math.Ceil(math.Log2(float64(len(vals)))))
	log.Info("generating mesh with %v links per endpoint", n)

	for i, v := range vals {
		for j := uint(0); j < n; j++ {
			i2 := (i + 1<<j) % len(vals)
			// make sure we don't connect to self
			if i == i2 {
				i2 = (i2 + 1) % len(vals)
			}

			res[v] = append(res[v], vals[i2])
		}
	}

	return res
}
