// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	log "minilog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type Command struct {
	ID int

	// true if the master should record responses to disk
	Record bool

	// run command in the background and return immediately
	Background bool

	// The command is a slice of strings with the first element being the
	// command, and any other elements as the arguments
	Command []string

	// Files to transfer to the client if type == COMMAND_EXEC | COMMAND_FILE_SEND
	// Any path given in a file specified here will be rooted at <BASE>/files
	FilesSend []string

	// Files to transfer back to the master if type == COMMAND_EXEC | COMMAND_FILE_RECV
	FilesRecv []string

	// Filter for clients to process commands. Not all fields in a client
	// must be set (wildcards), but all set fields must match for a command
	// to be processed. A client may match on one or more clients in the
	// slice, which allows for filters to be processed as a logical sum of
	// products.
	Filter []*Client

	// clients that have responded to this command
	// leave this private as we don't want to bother sending this
	// downstream
	checkedIn []string

	// conditions on which commands can expire
	ExpireClients  int
	ExpireStarted  time.Time
	ExpireDuration time.Duration
	ExpireTime     time.Time
}

type Response struct {
	// ID counter, must match the corresponding Command
	ID int

	UUID string

	// Names and data for uploaded files
	Files map[string][]byte

	// Output from responding command, if any
	Stdout string
	Stderr string
}

func filterString(filter []*Client) string {
	var ret string
	for _, f := range filter {
		if len(ret) != 0 {
			ret += " || "
		}
		ret += "( "
		var j []string
		if f.UUID != "" {
			j = append(j, "uuid="+f.UUID)
		}
		if f.Hostname != "" {
			j = append(j, "hostname="+f.Hostname)
		}
		if f.Arch != "" {
			j = append(j, "arch="+f.Arch)
		}
		if f.OS != "" {
			j = append(j, "os="+f.OS)
		}
		if len(f.IP) != 0 {
			for _, y := range f.IP {
				j = append(j, "ip="+y)
			}
		}
		if len(f.MAC) != 0 {
			for _, y := range f.MAC {
				j = append(j, "mac="+y)
			}
		}
		ret += strings.Join(j, " && ")
		ret += " )"
	}
	return ret
}

func (r *Ron) shouldRecord(id int) bool {
	r.commandLock.Lock()
	defer r.commandLock.Unlock()
	if c, ok := r.commands[id]; ok {
		return c.Record
	}
	return false
}

// periodically reap commands that meet expiry conditions
func (r *Ron) expireReaper() {
	for {
		time.Sleep(time.Duration(REAPER_RATE) * time.Second)
		log.Debugln("expireReaper")
		now := time.Now()
		r.commandLock.Lock()
		for k, v := range r.commands {
			if v.ExpireClients != 0 {
				if len(v.checkedIn) >= v.ExpireClients {
					log.Debug("expiring command %v after %v/%v checkins", k, len(v.checkedIn), v.ExpireClients)
					delete(r.commands, k)
				}
			} else if v.ExpireDuration != 0 {
				if time.Since(v.ExpireStarted) > v.ExpireDuration {
					log.Debug("expiring command %v after %v", k, v.ExpireDuration)
					delete(r.commands, k)
				}
			} else if !v.ExpireTime.IsZero() {
				if now.After(v.ExpireTime) {
					log.Debug("expiring command %v at time %v, now is %v", k, v.ExpireTime, now)
					delete(r.commands, k)
				}
			}
		}
		r.commandLock.Unlock()
	}
}

func (r *Ron) commandCheckIn(id int, uuid string) {
	log.Debug("commandCheckIn %v %v", id, uuid)

	r.commandLock.Lock()
	if c, ok := r.commands[id]; ok {
		c.checkedIn = append(c.checkedIn, uuid)
	}
	r.commandLock.Unlock()
}

func (r *Ron) getCommandID() int {
	log.Debugln("getCommandID")
	r.commandCounterLock.Lock()
	defer r.commandCounterLock.Unlock()
	r.commandCounter++
	id := r.commandCounter
	return id
}

func (r *Ron) getMaxCommandID() int {
	log.Debugln("getMaxCommandID")
	return r.commandCounter
}

func (r *Ron) checkMaxCommandID(id int) {
	log.Debugln("checkMaxCommandID")
	r.commandCounterLock.Lock()
	defer r.commandCounterLock.Unlock()
	if id > r.commandCounter {
		log.Debug("found higher ID %v", id)
		r.commandCounter = id
	}
}

func (r *Ron) DeleteCommand(id int) error {
	r.commandLock.Lock()
	defer r.commandLock.Unlock()
	if _, ok := r.commands[id]; ok {
		delete(r.commands, id)
		return nil
	} else {
		return fmt.Errorf("command %v not found", id)
	}
}

func (r *Ron) DeleteFiles(id int) error {
	r.commandLock.Lock()
	defer r.commandLock.Unlock()
	if _, ok := r.commands[id]; ok {
		path := filepath.Join(r.path, "responses", strconv.Itoa(id))
		err := os.RemoveAll(path)
		if err != nil {
			log.Errorln(err)
			return err
		}
		return nil
	} else {
		return fmt.Errorf("command %v not found", id)
	}
}

func (r *Ron) NewCommand(c *Command) int {
	c.ID = r.getCommandID()
	r.commandLock.Lock()
	r.commands[c.ID] = c
	r.commandLock.Unlock()
	return c.ID
}

func (r *Ron) Resubmit(id int) error {
	r.commandLock.Lock()
	defer r.commandLock.Unlock()
	if c, ok := r.commands[id]; ok {
		newcommand := &Command{
			ID:         r.getCommandID(),
			Record:     c.Record,
			Background: c.Background,
			Command:    c.Command,
			FilesSend:  c.FilesSend,
			FilesRecv:  c.FilesRecv,
			Filter:     c.Filter,
		}
		r.commands[newcommand.ID] = newcommand
		return nil
	} else {
		return fmt.Errorf("command %v not found", id)
	}
}

func (r *Ron) CommandSummary() string {
	r.commandLock.Lock()
	defer r.commandLock.Unlock()

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)

	fmt.Fprintf(w, "ID\tcommand\tclients checked in\trecord\tbackground\tsend files\treceive files\tfilter\n")
	for _, v := range r.commands {
		filter := filterString(v.Filter)
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\n", v.ID, v.Command, len(v.checkedIn), v.Record, v.Background, v.FilesSend, v.FilesRecv, filter)
	}

	w.Flush()

	return o.String()
}

func (r *Ron) encodeCommands() ([]byte, error) {
	log.Debugln("encodeCommands")
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(r.commands)
	if err != nil {
		log.Errorln(err)
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

func (r *Ron) clientCommands(newCommands map[int]*Command) {
	log.Debugln("clientCommands")
	cmds := make(map[int]*Command)

	var ids []int
	for k, _ := range newCommands {
		ids = append(ids, k)
	}
	sort.Ints(ids)

	maxCommandID := r.getMaxCommandID()
	for _, c := range ids {
		if newCommands[c].ID > maxCommandID {
			if !r.matchFilter(newCommands[c]) {
				continue
			}
			r.checkMaxCommandID(newCommands[c].ID)
			cmds[c] = newCommands[c]
		}
	}

	r.clientCommandQueue <- cmds
}

func (r *Ron) getFiles(files []string) {
	for _, v := range files {
		log.Debug("get file %v", v)
		path := filepath.Join(r.path, v)

		if _, err := os.Stat(path); err == nil {
			log.Debug("file %v already exists", v)
			continue
		}

		url := fmt.Sprintf("http://%v:%v/files/%v", r.parent, r.port, v)
		log.Debug("file get url %v", url)
		resp, err := http.Get(url)
		if err != nil {
			// TODO: should we retry?
			log.Errorln(err)
			continue
		}

		dir := filepath.Dir(path)
		err = os.MkdirAll(dir, os.FileMode(0770))
		if err != nil {
			log.Errorln(err)
			resp.Body.Close()
			continue
		}
		f, err := os.Create(path)
		if err != nil {
			log.Errorln(err)
			resp.Body.Close()
			continue
		}
		io.Copy(f, resp.Body)
		f.Close()
		resp.Body.Close()
	}
}

func (r *Ron) matchFilter(c *Command) bool {
	if len(c.Filter) == 0 {
		return true
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	for _, v := range c.Filter {
		if v.UUID != "" && v.UUID != r.UUID {
			log.Debug("failed match on UUID %v %v", v.UUID, r.UUID)
			continue
		}
		if v.Hostname != "" && v.Hostname != hostname {
			log.Debug("failed match on hostname %v %v", v.Hostname, hostname)
			continue
		}
		if v.Arch != "" && v.Arch != runtime.GOARCH {
			log.Debug("failed match on arch %v %v", v.Arch, runtime.GOARCH)
			continue
		}
		if v.OS != "" && v.OS != runtime.GOOS {
			log.Debug("failed match on os %v %v", v.OS, runtime.GOOS)
			continue
		}

		macs, ips := getNetworkInfo()

		if len(v.IP) != 0 {
			// special case, IPs can match on CIDRs as well as full IPs
			match := false
		MATCH_FILTER_IP:
			for _, i := range v.IP {
				for _, ip := range ips {
					if i == ip || matchCIDR(i, ip) {
						log.Debug("match on ip %v %v", i, ip)
						match = true
						break MATCH_FILTER_IP
					}
					log.Debug("failed match on ip %v %v", i, ip)
				}
			}
			if !match {
				continue
			}
		}
		if len(v.MAC) != 0 {
			match := false
		MATCH_FILTER_MAC:
			for _, m := range v.MAC {
				for _, mac := range macs {
					if mac == m {
						log.Debug("match on mac %v %v", m, mac)
						match = true
						break MATCH_FILTER_MAC
					}
					log.Debug("failed match on mac %v %v", m, mac)
				}
			}
			if !match {
				continue
			}
		}
		return true
	}
	return false
}

func matchCIDR(cidr string, ip string) bool {
	if !strings.Contains(cidr, "/") {
		return false
	}

	d := strings.Split(cidr, "/")
	log.Debugln("subnet ", d)
	if len(d) != 2 {
		return false
	}
	if !isIPv4(d[0]) {
		return false
	}

	netmask, err := strconv.Atoi(d[1])
	if err != nil {
		return false
	}
	network := toInt32(d[0])
	ipmask := toInt32(ip) & ^((1 << uint32(32-netmask)) - 1)
	log.Debug("got network %v and ipmask %v", network, ipmask)
	if ipmask == network {
		return true
	}
	return false
}

func isIPv4(ip string) bool {
	d := strings.Split(ip, ".")
	if len(d) != 4 {
		return false
	}

	for _, v := range d {
		octet, err := strconv.Atoi(v)
		if err != nil {
			return false
		}
		if octet < 0 || octet > 255 {
			return false
		}
	}

	return true
}

func toInt32(ip string) uint32 {
	d := strings.Split(ip, ".")

	var ret uint32
	for _, v := range d {
		octet, err := strconv.Atoi(v)
		if err != nil {
			return 0
		}

		ret <<= 8
		ret |= uint32(octet) & 0x000000ff
	}
	return ret
}
