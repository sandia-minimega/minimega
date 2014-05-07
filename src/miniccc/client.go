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
	volatile  bool
	OSVer     string
	CSDVer    string
	EditionID string
}

var (
	CID                int64
	responseQueue      []*Response
	responseQueueLock  sync.Mutex
	clientCommandQueue chan []*Command
	OSVer              string
	CSDVer             string
	EditionID          string
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
	getVersion()

	go clientCommandProcessor()

	log.Debug("CID: %v", CID)
}

//Populate OSVer (e.g. "Windows 7")
func getOSVer() {
	var fullVersion string

	//Get CurrentVersion
	cmd := exec.Command("reg", "query",
		"HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
		"/v", "CurrentVersion")
	cvBytes, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnln("failed reg query: CurrentVersion")
	}
	cvStr := strings.Split(string(cvBytes), "    ")
	currentVersion := strings.TrimSpace(cvStr[len(cvStr)-1])

	//Get CurrentBuild
	cmd = exec.Command("reg", "query",
		"HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
		"/v", "CurrentBuild")

	cbBytes, err := cmd.CombinedOutput()

	if err != nil {
		log.Warnln("failed reg query: CurrentBuild")
		fullVersion = currentVersion
	} else {
		cbStr := strings.Split(string(cbBytes), "    ")
		currentBuild := strings.TrimSpace(cbStr[len(cbStr)-1])

		fullVersion = currentVersion + "." + currentBuild
	}

	switch fullVersion {
	case "1.04":
		OSVer = "Windows 1.0"
	case "2.11":
		OSVer = "Windows 2.0"
	case "3":
		OSVer = "Windows 3.0"
	case "3.11":
		OSVer = "Windows for Workgroups 3.11"
	case "2250":
		OSVer = "Whistler Server"
	case "2257":
		OSVer = "Whistler Server"
	case "2267":
		OSVer = "Whistler Server"
	case "2410":
		OSVer = "Whistler Server"
	case "3.10.528":
		OSVer = "Windows NT 3.1"
	case "3.5.807":
		OSVer = "Windows NT Workstation 3.5"
	case "3.51.1057":
		OSVer = "Windows NT Workstation 3.51"
	case "4.0.1381":
		OSVer = "Windows Workstation 4.0"
	case "4.0.950":
		OSVer = "Windows 95"
	case "4.00.950":
		OSVer = "Windows 95"
	case "4.00.1111":
		OSVer = "Windows 95"
	case "4.03.1212-1214":
		OSVer = "Windows 95"
	case "4.03.1214":
		OSVer = "Windows 95"
	case "4.1.1998":
		OSVer = "Windows 98"
	case "4.1.2222":
		OSVer = "Windows 98"
	case "4.90.2476":
		OSVer = "Windows Millenium"
	case "4.90.3000":
		OSVer = "Windows Me"
	case "5.00.1515":
		OSVer = "Windows NT 5.00"
	case "5.00.2031":
		OSVer = "Windows 2000"
	case "5.00.2128":
		OSVer = "Windows 2000"
	case "5.00.2183":
		OSVer = "Windows 2000"
	case "5.00.2195":
		OSVer = "Windows 2000"
	case "5.0.2195":
		OSVer = "Windows 2000"
		EditionID = "Professional"
	case "5.1.2505":
		OSVer = "Windows XP"
	case "5.1.2600":
		OSVer = "Windows XP"
	case "5.2.3790":
		OSVer = "Windows XP"
		EditionID = "Professional"
		//      Conflicts with Windows XP.
		//	case "5.2.3790": OSVer = "Windows Home Server"
		//	case "5.2.3790": OSVer = "Windows Server 2003"
	case "5.2.3541":
		OSVer = "Windows .NET Server"
	case "5.2.3590":
		OSVer = "Windows .NET Server"
	case "5.2.3660":
		OSVer = "Windows .NET Server"
	case "5.2.3718":
		OSVer = "Windows .NET Server 2003"
	case "5.2.3763":
		OSVer = "Windows Server 2003"
	case "6.0.5048":
		OSVer = "Windows Longhorn"
	case "6.0.5112":
		OSVer = "Windows Vista"
	case "6.0.5219":
		OSVer = "Windows Vista"
	case "6.0.5259":
		OSVer = "Windows Vista"
	case "6.0.5270":
		OSVer = "Windows Vista"
	case "6.0.5308":
		OSVer = "Windows Vista"
	case "6.0.5342":
		OSVer = "Windows Vista"
	case "6.0.5381":
		OSVer = "Windows Vista"
	case "6.0.5384":
		OSVer = "Windows Vista"
	case "6.0.5456":
		OSVer = "Windows Vista"
	case "6.0.5472":
		OSVer = "Windows Vista"
	case "6.0.5536":
		OSVer = "Windows Vista"
	case "6.0.5600":
		OSVer = "Windows Vista"
	case "6.0.5700":
		OSVer = "Windows Vista"
	case "6.0.5728":
		OSVer = "Windows Vista"
	case "6.0.5744":
		OSVer = "Windows Vista"
	case "6.0.5808":
		OSVer = "Windows Vista"
	case "6.0.5824":
		OSVer = "Windows Vista"
	case "6.0.5840":
		OSVer = "Windows Vista"
	case "6.0.6000":
		OSVer = "Windows Vista"
	case "6.0.6001":
		OSVer = "Windows Server 2008"
	case "6.0.6002":
		OSVer = "Windows Vista"
	case "6.1.7600":
		OSVer = "Windows 7"
		//      Conflicts with Windows 7.  Need more granularity (.16385)
		//	case "6.1.7600": OSVer = "Windows Server 2008 R2, RTM (Release to Manufacturing)"
	case "6.1.7601":
		OSVer = "Windows 7"
		CSDVer = "Service Pack 1"
	case "6.2.9200":
		OSVer = "Windows 8"
		//	Conflicts with Windows 8.  Not sure how to tell these apart
		//	case "6.2.9200": OSVer = "Windows Server 2012"
	case "6.2.8102":
		OSVer = "Windows Server 2012"
	case "6.3.9600":
		OSVer = "Windows 8.1"
	}
}

//Populate CSDVer (e.g. "Service Pack 1")
func getCSDVer() {
	//if CSDVer was set while getting OSVer, skip this function
	if CSDVer != "" {
		return
	}

	CSDVer = "none"

	cmd := exec.Command("reg", "query",
		"HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
		"/v", "CSDVersion")

	csdBytes, err := cmd.CombinedOutput()

	if err != nil {
		log.Warnln("failed reg query: CSDVersion")
	} else {
		csdStr := strings.Split(string(csdBytes), "    ")
		CSDVer = strings.TrimSpace(csdStr[len(csdStr)-1])
	}
}

//Populate EditionID (e.g. Enterprise)
func getEditionID() {
	//if EditionID was set while getting OSVer, skip this function
	if EditionID != "" {
		return
	}
	EditionID = "none"
	cmd := exec.Command("reg", "query",
		"HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
		"/v", "EditionID")

	eidBytes, err := cmd.CombinedOutput()

	if err != nil {
		log.Warnln("failed reg query: EditionID")
	}
	eidStr := strings.Split(string(eidBytes), "    ")
	EditionID = strings.TrimSpace(eidStr[len(eidStr)-1])
}

func getWindowsVersion() {
	getOSVer()
	getCSDVer()
	getEditionID()
}

func getVersion() {
	switch runtime.GOOS {
	case "windows":
		getWindowsVersion()
	}
}

func clientHeartbeat() *hb {
	log.Debugln("clientHeartbeat")

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}

	c := &Client{
		CID:       CID,
		Arch:      runtime.GOARCH,
		OS:        runtime.GOOS,
		Hostname:  hostname,
		OSVer:     OSVer,
		CSDVer:    CSDVer,
		EditionID: EditionID,
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
			// trim the cidr from the end
			var ip string
			i := strings.Split(w.String(), "/")
			if len(i) != 2 {
				if !isIPv4(w.String()) {
					log.Error("malformed ip: %v", i, w)
					continue
				}
				ip = w.String()
			} else {
				ip = i[0]
			}
			log.Debug("found ip: %v", ip)
			ips = append(ips, ip)
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
		if newCommands[c].ID > maxCommandID {
			if !matchFilter(newCommands[c]) {
				continue
			}
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
			log.Debug("failed match on CID %v %v", v.CID, CID)
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
		} else if runtime.GOOS == "windows" {
			if v.OSVer != "" && v.OSVer != OSVer {
				log.Debug("failed match on os version %v %v", v.OSVer, OSVer)
				continue
			}
			if v.CSDVer != "" && v.CSDVer != CSDVer {
				log.Debug("failed match on CSDVersion %v %v", v.CSDVer, CSDVer)
				continue
			}
			if v.EditionID != "" && v.EditionID != EditionID {
				log.Debug("failed match on EditionID %v %v", v.EditionID, EditionID)
				continue
			}
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

func clientCommandProcessor() {
	log.Debugln("clientCommandProcessor")
	for {
		c := <-clientCommandQueue
		for _, v := range c {
			maxCommandID := getMaxCommandID()
			if v.ID <= maxCommandID {
				log.Info("processing (skipping) command %v again, is command still in flight?", v.ID)
				continue
			}

			checkMaxCommandID(v.ID)
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
