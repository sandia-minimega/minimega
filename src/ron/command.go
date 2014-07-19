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
		path := fmt.Sprintf("%v/responses/%v", r.path, id)
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

	fmt.Fprintf(w, "ID\tcommand\trecord\tbackground\tsend files\treceive files\tfilter\n")
	for _, v := range r.commands {
		filter := filterString(v.Filter)
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\t%v\n", v.ID, v.Command, v.Record, v.Background, v.FilesSend, v.FilesRecv, filter)
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

//func handleNewCommand(w http.ResponseWriter, r *http.Request) {
//	log.Debugln("handleNewCommand")
//
//	// if no args, then present the new command dialog, otherwise try to parse the input
//	commandType := r.FormValue("type")
//	var resp string
//
//	ec := r.FormValue("expire_responses")
//	ed := r.FormValue("expire_duration")
//	et := r.FormValue("expire_time")
//
//	expireClients, err := strconv.Atoi(ec)
//	if err != nil {
//		if ec != "" {
//			log.Errorln(err)
//		}
//		expireClients = 0
//	}
//	log.Debug("got expireClients %v", expireClients)
//
//	expireDuration, err := time.ParseDuration(ed)
//	if err != nil {
//		if ed != "" {
//			log.Errorln(err)
//		}
//		expireDuration = time.Duration(0)
//	}
//	log.Debug("got expireDuration %v", expireDuration)
//
//	now := time.Now()
//	expireTime, err := time.Parse(time.Kitchen, et)
//	if err != nil {
//		if et != "" {
//			log.Errorln(err)
//		}
//		expireTime = time.Time{}
//	} else {
//		expireTime = time.Date(now.Year(), now.Month(), now.Day(), expireTime.Hour(), expireTime.Minute(), expireTime.Second(), 0, now.Location())
//	}
//	log.Debug("got expireTime %v", expireTime)
//
//	log.Debug("got type %v", commandType)
//
//	switch commandType {
//	case "exec":
//		commandCmd := r.FormValue("command")
//		if commandCmd == "" {
//			resp = "<html>no command specified</html>"
//		} else {
//			commandFilesSend := r.FormValue("filesend")
//			commandFilesRecv := r.FormValue("filerecv")
//			commandRecord := r.FormValue("record")
//			var record bool
//			if commandRecord == "record" {
//				record = true
//			}
//			c := &Command{
//				Type:           COMMAND_EXEC,
//				Record:         record,
//				ID:             getCommandID(),
//				Command:        fieldsQuoteEscape(commandCmd),
//				FilesSend:      strings.Fields(commandFilesSend),
//				FilesRecv:      strings.Fields(commandFilesRecv),
//				Filter:         getFilter(r),
//				ExpireClients:  expireClients,
//				ExpireStarted:  time.Now(),
//				ExpireDuration: expireDuration,
//				ExpireTime:     expireTime,
//			}
//			log.Debug("generated command %v", c)
//			commandLock.Lock()
//			commands[c.ID] = c
//			commandLock.Unlock()
//			resp = fmt.Sprintf("<html>command %v submitted</html", c.ID)
//		}
//	case "filesend":
//		commandFilesSend := r.FormValue("filesend")
//		if commandFilesSend == "" {
//			resp = "<html>no files specified</html>"
//		} else {
//			commandRecord := r.FormValue("record")
//			var record bool
//			if commandRecord == "record" {
//				record = true
//			}
//			c := &Command{
//				Type:           COMMAND_FILE_SEND,
//				Record:         record,
//				ID:             getCommandID(),
//				FilesSend:      strings.Fields(commandFilesSend),
//				Filter:         getFilter(r),
//				ExpireClients:  expireClients,
//				ExpireStarted:  time.Now(),
//				ExpireDuration: expireDuration,
//				ExpireTime:     expireTime,
//			}
//			log.Debug("generated command %v", c)
//			commandLock.Lock()
//			commands[c.ID] = c
//			commandLock.Unlock()
//			resp = fmt.Sprintf("<html>command %v submitted</html", c.ID)
//		}
//	case "filerecv":
//		commandFilesRecv := r.FormValue("filerecv")
//		if commandFilesRecv == "" {
//			resp = "<html>no files specified</html>"
//		} else {
//			commandRecord := r.FormValue("record")
//			var record bool
//			if commandRecord == "record" {
//				record = true
//			}
//			c := &Command{
//				Type:           COMMAND_FILE_RECV,
//				Record:         record,
//				ID:             getCommandID(),
//				FilesRecv:      strings.Fields(commandFilesRecv),
//				Filter:         getFilter(r),
//				ExpireClients:  expireClients,
//				ExpireStarted:  time.Now(),
//				ExpireDuration: expireDuration,
//				ExpireTime:     expireTime,
//			}
//			log.Debug("generated command %v", c)
//			commandLock.Lock()
//			commands[c.ID] = c
//			commandLock.Unlock()
//			resp = fmt.Sprintf("<html>command %v submitted</html", c.ID)
//		}
//	case "log":
//		commandLogLevel := r.FormValue("loglevel")
//		if commandLogLevel == "" {
//			resp = "<html>no log level specified</html>"
//		} else {
//			commandRecord := r.FormValue("record")
//			var record bool
//			if commandRecord == "record" {
//				record = true
//			}
//			c := &Command{
//				Type:           COMMAND_LOG,
//				Record:         record,
//				ID:             getCommandID(),
//				LogLevel:       commandLogLevel,
//				LogPath:        r.FormValue("logpath"),
//				Filter:         getFilter(r),
//				ExpireClients:  expireClients,
//				ExpireStarted:  time.Now(),
//				ExpireDuration: expireDuration,
//				ExpireTime:     expireTime,
//			}
//			log.Debug("generated command %v", c)
//			commandLock.Lock()
//			commands[c.ID] = c
//			commandLock.Unlock()
//			resp = fmt.Sprintf("<html>command %v submitted</html", c.ID)
//		}
//	default:
//		resp = `
//			<html>
//				<form method=post action=/command/new>
//					Command type: <select name=type>
//						<option selected value=exec>Execute</option>
//						<option value=filesend>Send Files</option>
//						<option value=filerecv>Receive Files</option>
//						<option value=log>Change log level</option>
//					</select>
//					<br>
//					<input type=checkbox name=record value=record>Record Responses
//					<br>
//					Command: <input type=text name=command>
//					<br>
//					Files -> client (space delimited) <input type=text name=filesend>
//					<br>
//					Files <- client (space delimited) <input type=text name=filerecv>
//					<br>
//					New log level: <select name=loglevel>
//						<option value=debug>Debug</option>
//						<option value=info>Info</option>
//						<option selected value=warn>Warn</option>
//						<option value=error>Error</option>
//						<option value=fatal>Fatal</option>
//					</select>
//					<br>
//					Log file path: <input type=text name=logpath>
//					<br>
//					Filter (blank fields are wildcard):
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;CID: <input type=text name=filter_cid>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;Hostname: <input type=text name=filter_hostname>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;Arch: <input type=text name=filter_arch>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;OS: <input type=text name=filter_os>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;OS Version: <select name=filter_osver>
//					<option value=""></option>
//					<option value="Windows 7">Windows 7</option>
//					<option value="Windows XP">Windows XP</option>
//					<option value="Windows Vista">Windows Vista</option>
//					<option value="Windows 8">Windows 8</option>
//					<option value="Windows 8.1">Windows 8.1</option>
//					<option value="Windows Server 2012">Windows Server 2012</option>
//					<option value="Windows Server 2008">Windows Server 2008</option>
//					<option value="Windows Server 2003">Windows Server 2003</option>
//					<option value="Windows 2000">Windows 2000</option>
//					<option value="Windows Longhorn">Windows Longhorn</option>
//					<option value="Windows .NET Server 2003">Windows .NET Server 2003</option>
//					<option value="Windows .NET Server">Windows .NET Server 2003</option>
//					<option value="Windows NT 5.00">Windows NT 5.00</option>
//					<option value="Windows Me">Windows Me</option>
//					<option value="Windows 98">Windows 98</option>
//					<option value="Windows 95">Windows 95</option>
//					</select>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;CSD Version: <select name=filter_csdver>
//					<option value=""></option>
//					<option value="none">None</option>
//					<option value="Service Pack 1">Service Pack 1</option>
//					<option value="Service Pack 2">Service Pack 2</option>
//					<option value="Service Pack 3">Service Pack 3</option>
//					</select>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;Edition ID: <select name=filter_editionid>
//					<option value=""></option>
//					<option value="Starter">Starter</option>
//					<option value="Home Basic">Home Basic</option>
//					<option value="Home Premium">Home Premium</option>
//					<option value="Professional">Professional</option>
//					<option value="Enterprise">Enterprise</option>
//					<option value="Ultimate">Ultimate</option>
//					</select>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;IP (IP or CIDR list, space delimited): <input type=text name=filter_ip>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;MAC (space delimited): <input type=text name=filter_mac>
//					Command Expiry (blank fields are unused):
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;Number of responses: <input type=text name=expire_responses>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;Duration: <input type=text name=expire_duration>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Duration examples: (300s, 2h45m). Valid units are "s", "m", "h"
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;Time: <input type=text name=expire_time>
//					<br>
//					&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Time must be in the form of "3:04PM"
//					<br>
//					<input type=submit value=Submit>
//				</form>
//			</html>`
//	}
//
//	w.Write([]byte(resp))
//}
//
//func getFilter(r *http.Request) []*Client {
//	cid := r.FormValue("filter_cid")
//	cidInt, err := strconv.ParseInt(cid, 10, 64)
//	if err != nil {
//		cidInt = 0
//	}
//	host := r.FormValue("filter_hostname")
//	arch := r.FormValue("filter_arch")
//	os := r.FormValue("filter_os")
//	osver := r.FormValue("filter_osver")
//	csdver := r.FormValue("filter_csdver")
//	editionid := r.FormValue("filter_editionid")
//	ip := r.FormValue("filter_ip")
//	mac := r.FormValue("filter_mac")
//
//	ips := strings.Fields(ip)
//	macs := strings.Fields(mac)
//
//	return []*Client{&Client{
//		CID:       cidInt,
//		Hostname:  host,
//		Arch:      arch,
//		OS:        os,
//		OSVer:     osver,
//		CSDVer:    csdver,
//		EditionID: editionid,
//		IP:        ips,
//		MAC:       macs,
//	}}
//}
//
//func handleDeleteCommand(w http.ResponseWriter, r *http.Request) {
//	log.Debugln("handleDeleteCommand")
//	id := r.FormValue("id")
//	val, err := strconv.Atoi(id)
//	if err != nil {
//		log.Errorln(err)
//		w.Write([]byte(err.Error()))
//		return
//	}
//	resp := commandDelete(val)
//	resp = fmt.Sprintf("<html>%v</html>", resp)
//	w.Write([]byte(resp))
//}
//
//func handleDeleteFiles(w http.ResponseWriter, r *http.Request) {
//	log.Debugln("handleDeleteFiles")
//	id := r.FormValue("id")
//	val, err := strconv.Atoi(id)
//	if err != nil {
//		log.Errorln(err)
//		w.Write([]byte(err.Error()))
//		return
//	}
//	resp := commandDeleteFiles(val)
//	resp = fmt.Sprintf("<html>%v</html>", resp)
//	w.Write([]byte(resp))
//}
//
//func handleResubmit(w http.ResponseWriter, r *http.Request) {
//	log.Debugln("handleResubmit")
//	id := r.FormValue("id")
//	val, err := strconv.Atoi(id)
//	if err != nil {
//		log.Errorln(err)
//		w.Write([]byte(err.Error()))
//		return
//	}
//	resp := commandResubmit(val)
//	resp = fmt.Sprintf("<html>%v</html>", resp)
//	w.Write([]byte(resp))
//}

func (r *Ron) clientCommands(newCommands map[int]*Command) {
	log.Debugln("clientCommands")
	r.clientCommandQueue <- newCommands
}

func (r *Ron) getFiles(files []string) {
	for _, v := range files {
		log.Debug("get file %v", v)
		path := fmt.Sprintf("%v/%v", r.path, v)

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

// Return a slice of strings, split on whitespace, not unlike strings.Fields(),
// except that quoted fields are grouped.
// 	Example: a b "c d"
// 	will return: ["a", "b", "c d"]
func fieldsQuoteEscape(input string) []string {
	f := strings.Fields(input)
	var ret []string
	trace := false
	temp := ""
	for _, v := range f {
		if trace {
			if strings.HasSuffix(v, "\"") {
				trace = false
				temp += " " + v[:len(v)-1]
				ret = append(ret, temp)
			} else {
				temp += " " + v
			}
		} else if strings.HasPrefix(v, "\"") {
			trace = true
			temp = v[1:]

		} else {
			ret = append(ret, v)
		}
	}
	return ret
}
