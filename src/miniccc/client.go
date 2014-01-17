package main

import (
	"bytes"
	"encoding/base64"
	"math/rand"
	log "minilog"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Client struct {
	CID       string
	Hostname  string
	Arch      string
	OS        string
	IP        []string
	MAC       []string
	Checkin   time.Time
	Responses []*Response
}

var (
	CID                string
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
	b := make([]byte, 16)
	for i, _ := range b {
		b[i] = byte(r.Int())
	}

	go clientCommandProcessor()

	CID = base64.StdEncoding.EncodeToString(b)
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

	// process network info
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
		c.MAC = append(c.MAC, v.HardwareAddr.String())
		addrs, err := v.Addrs()
		if err != nil {
			log.Fatalln(err)
		}
		for _, w := range addrs {
			log.Debug("found ip: %v", w)
			c.IP = append(c.IP, w.String())
		}
	}

	me := make(map[string]*Client)
	me[CID] = c
	h := &hb{
		ID:           CID,
		Clients:      me,
		MaxCommandID: getMaxCommandID(),
	}
	log.Debug("client heartbeat %v", h)
	return h
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
		// TODO: allow filters here
		if newCommands[c].ID > maxCommandID {
			myCommands = append(myCommands, newCommands[c])
		}
	}

	clientCommandQueue <- myCommands
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
			case COMMAND_FILE_RECV:
			case COMMAND_LOG:
			default:
				log.Error("invalid command type %v", v.Type)
			}
		}
	}
}

func clientCommandExec(c *Command) {
	log.Debug("clientCommandExec %v", c.ID)
	resp := &Response{
		ID: c.ID,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var args []string
	if len(c.Command) > 1 {
		args = c.Command[1:]
	}

	path, err := exec.LookPath(c.Command[0])
	if err != nil {
		log.Errorln(err)
		resp.Stderr = err.Error()
	} else {
		cmd := &exec.Cmd{
			Path:   path,
			Args:   args,
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

	responseQueueLock.Lock()
	responseQueue = append(responseQueue, resp)
	checkMaxCommandID(c.ID)
	responseQueueLock.Unlock()
}
