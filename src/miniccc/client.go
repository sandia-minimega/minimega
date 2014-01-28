package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	log "minilog"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	CID       int64
	Hostname  string
	Arch      string
	OS        string
	IP        []string
	MAC       []string
	Checkin   time.Time
	Responses []*Response
}

var (
	CID                int64
	responseQueue      []*Response
	responseQueueLock  sync.Mutex
	clientCommandQueue chan []*Command
)

func init() {
	clientCommandQueue = make(chan []*Command, 1024)
}

func clientSetup() {
	log.Debugln("clientSetup")

	// generate a random byte slice
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	CID = r.Int63()

	go clientCommandProcessor()

	log.Debug("CID: %v", CID)
}

func clientHeartbeat() *hb {
	log.Debugln("clientHeartbeat")

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	c := &Client{
		CID:      CID,
		Arch:     runtime.GOARCH,
		OS:       runtime.GOOS,
		Hostname: hostname,
	}

	// attach any command responses and clear the response queue
	responseQueueLock.Lock()
	c.Responses = responseQueue
	responseQueue = []*Response{}
	responseQueueLock.Unlock()

	macs, ips := getNetworkInfo()
	c.MAC = macs
	c.IP = ips

	me := make(map[int64]*Client)
	me[CID] = c
	h := &hb{
		ID:           CID,
		Clients:      me,
		MaxCommandID: getMaxCommandID(),
	}
	log.Debug("client heartbeat %v", h)
	return h
}

func getNetworkInfo() ([]string, []string) {
	// process network info
	var macs []string
	var ips []string

	ints, err := net.Interfaces()
	if err != nil {
		log.Fatalln(err)
	}
	for _, v := range ints {
		if v.HardwareAddr.String() == "" {
			// skip localhost and other weird interfaces
			continue
		}
		log.Debug("found mac: %v", v.HardwareAddr)
		macs = append(macs, v.HardwareAddr.String())
		addrs, err := v.Addrs()
		if err != nil {
			log.Fatalln(err)
		}
		for _, w := range addrs {
			log.Debug("found ip: %v", w)
			ips = append(ips, w.String())
		}
	}
	return macs, ips
}

func clientCommands(newCommands map[int]*Command) {
	// run any commands that apply to us, they'll inject their responses
	// into the response queue

	var ids []int
	for k, _ := range newCommands {
		ids = append(ids, k)
	}
	sort.Ints(ids)

	var myCommands []*Command

	maxCommandID := getMaxCommandID()
	for _, c := range ids {
		if !matchFilter(newCommands[c]) {
			continue
		}

		if newCommands[c].ID > maxCommandID {
			myCommands = append(myCommands, newCommands[c])
		}
	}

	clientCommandQueue <- myCommands
}

func matchFilter(c *Command) bool {
	if len(c.Filter) == 0 {
		return true
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	for _, v := range c.Filter {
		if v.CID != 0 && v.CID != CID {
			continue
		}
		if v.Hostname != "" && v.Hostname != hostname {
			continue
		}
		if v.Arch != "" && v.Arch != runtime.GOARCH {
			continue
		}
		if v.OS != "" && v.OS != runtime.GOOS {
			continue
		}

		macs, ips := getNetworkInfo()

		if len(v.IP) != 0 {
			// special case, IPs can match on CIDRs as well as full IPs
		MATCH_FILTER_IP:
			for _, i := range v.IP {
				for _, ip := range ips {
					if i == ip || matchCIDR(i, ip) {
						break MATCH_FILTER_IP
					}
				}
			}
		}
		if len(v.MAC) != 0 {
		MATCH_FILTER_MAC:
			for _, m := range v.MAC {
				for _, mac := range macs {
					if mac == m {
						break MATCH_FILTER_MAC
					}
				}
			}
		}
		return true
	}
	return false
}

func matchCIDR(cidr string, ip string) bool {
	if !strings.Contains(ip, "/") {
		return false
	}

	d := strings.Split(ip, "/")
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

func clientCommandProcessor() {
	log.Debugln("clientCommandProcessor")
	for {
		c := <-clientCommandQueue
		for _, v := range c {
			log.Debug("processing command %v", v.ID)
			switch v.Type {
			case COMMAND_EXEC:
				clientCommandExec(v)
			case COMMAND_FILE_SEND:
				clientFilesSend(v)
			case COMMAND_FILE_RECV:
				clientFilesRecv(v)
			case COMMAND_LOG:
				clientCommandLog(v)
			default:
				log.Error("invalid command type %v", v.Type)
			}
		}
	}
}

func queueResponse(r *Response) {
	responseQueueLock.Lock()
	responseQueue = append(responseQueue, r)
	checkMaxCommandID(r.ID)
	responseQueueLock.Unlock()
}

func clientCommandLog(c *Command) {
	log.Debug("clientCommandExec %v", c.ID)
	resp := &Response{
		ID: c.ID,
	}
	err := logChange(c.LogLevel, c.LogPath)
	if err != nil {
		resp.Stderr = err.Error()
	} else {
		resp.Stdout = fmt.Sprintf("log level changed to %v", c.LogLevel)
		if c.LogPath == "" {
			resp.Stdout += fmt.Sprintf("\nlog path cleared\n")
		} else {
			resp.Stdout += fmt.Sprintf("\nlog path set to %v\n", c.LogPath)
		}
	}

	queueResponse(resp)
}

func clientFilesSend(c *Command) {
	log.Debug("clientFilesSend %v", c.ID)
	resp := &Response{
		ID: c.ID,
	}

	// get any files needed for the command
	if len(c.FilesSend) != 0 {
		commandGetFiles(c.FilesSend)
	}

	resp.Stdout = "files received"

	queueResponse(resp)
}

func clientFilesRecv(c *Command) {
	log.Debug("clientFilesRecv %v", c.ID)
	resp := &Response{
		ID: c.ID,
	}

	if len(c.FilesRecv) != 0 {
		resp.Files = prepareRecvFiles(c.FilesRecv)
	}

	queueResponse(resp)
}

func prepareRecvFiles(files []string) map[string][]byte {
	log.Debug("prepareRecvFiles %v", files)
	r := make(map[string][]byte)
	for _, f := range files {
		d, err := ioutil.ReadFile(f)
		if err != nil {
			log.Errorln(err)
			continue
		}
		r[f] = d
	}
	return r
}

func clientCommandExec(c *Command) {
	log.Debug("clientCommandExec %v", c.ID)
	resp := &Response{
		ID: c.ID,
	}

	// get any files needed for the command
	if len(c.FilesSend) != 0 {
		commandGetFiles(c.FilesSend)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	path, err := exec.LookPath(c.Command[0])
	if err != nil {
		log.Errorln(err)
		resp.Stderr = err.Error()
	} else {
		cmd := &exec.Cmd{
			Path:   path,
			Args:   c.Command,
			Env:    nil,
			Dir:    "",
			Stdout: &stdout,
			Stderr: &stderr,
		}
		log.Debug("executing %v", strings.Join(c.Command, " "))
		err := cmd.Run()
		if err != nil {
			log.Errorln(err)
			return
		}
		resp.Stdout = stdout.String()
		resp.Stderr = stderr.String()
	}

	if len(c.FilesRecv) != 0 {
		resp.Files = prepareRecvFiles(c.FilesRecv)
	}

	queueResponse(resp)
}
