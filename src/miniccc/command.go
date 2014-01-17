package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	log "minilog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	COMMAND_EXEC = iota
	COMMAND_FILE_SEND
	COMMAND_FILE_RECV
	COMMAND_LOG
)

type Command struct {
	ID int

	Type int

	// true if the master should record responses to disk
	Record bool

	// Command to run if type == COMMAND_EXEC
	// The command is a slice of strings with the first element being the
	// command, and any other elements as the arguments
	Command []string

	// Files to transfer to the client if type == COMMAND_EXEC | COMMAND_FILE_SEND
	// Any path given in a file specified here will be rooted at <BASE>/files
	FilesSend []string

	// Files to transfer back to the master if type == COMMAND_EXEC | COMMAND_FILE_RECV
	FilesRecv []string

	// Log level to set if type == COMMAND_LOG
	LogLevel string

	// File to output logging to if type == COMMAND_LOG
	// File logging will be disabled if LogPath == ""
	LogPath string

	// clients that have responded to this command
	// leave this private as we don't want to bother sending this
	// downstream
	checkedIn []int64
}

type Response struct {
	// ID counter, must match the corresponding Command
	ID int

	// Names and data for uploaded files
	Files map[string][]byte

	// Output from responding command, if any
	Stdout string
	Stderr string
}

var (
	commands           map[int]*Command
	commandCounter     int
	commandLock        sync.Mutex
	commandCounterLock sync.Mutex
)

func init() {
	commands = make(map[int]*Command)
}

func commandCheckIn(id int, cid int64) {
	commandLock.Lock()
	if c, ok := commands[id]; ok {
		c.checkedIn = append(c.checkedIn, cid)
	}
	commandLock.Unlock()
}

func getCommandID() int {
	log.Debugln("getCommandID")
	commandCounterLock.Lock()
	defer commandCounterLock.Unlock()
	commandCounter++
	id := commandCounter
	return id
}

func getMaxCommandID() int {
	log.Debugln("getMaxCommandID")
	return commandCounter
}

func checkMaxCommandID(id int) {
	log.Debugln("checkMaxCommandID")
	commandCounterLock.Lock()
	defer commandCounterLock.Unlock()
	if id > commandCounter {
		log.Debug("found higher ID %v", id)
		commandCounter = id
	}
}

func commandDelete(id int) string {
	commandLock.Lock()
	defer commandLock.Unlock()
	if _, ok := commands[id]; ok {
		delete(commands, id)
		return fmt.Sprintf("command %v deleted", id)
	} else {
		return fmt.Sprintf("command %v not found", id)
	}
}

func commandDeleteFiles(id int) string {
	commandLock.Lock()
	defer commandLock.Unlock()
	if _, ok := commands[id]; ok {
		path := fmt.Sprintf("%v/responses/%v", *f_base, id)
		err := os.RemoveAll(path)
		if err != nil {
			log.Errorln(err)
			return err.Error()
		}
		return fmt.Sprintf("command %v files deleted", id)
	} else {
		return fmt.Sprintf("command %v not found", id)
	}
}

func commandResubmit(id int) string {
	commandLock.Lock()
	defer commandLock.Unlock()
	if c, ok := commands[id]; ok {
		newcommand := &Command{
			ID:        getCommandID(),
			Type:      c.Type,
			Record:    c.Record,
			Command:   c.Command,
			FilesSend: c.FilesSend,
			FilesRecv: c.FilesRecv,
			LogLevel:  c.LogLevel,
			LogPath:   c.LogPath,
		}
		commands[newcommand.ID] = newcommand
		return fmt.Sprintf("command %v resubmitted as command %v", id, newcommand.ID)
	} else {
		return fmt.Sprintf("command %v not found", id)
	}
}

func encodeCommands() []byte {
	log.Debugln("encodeCommands")
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(commands)
	if err != nil {
		log.Errorln(err)
		return []byte{}
	}
	return buf.Bytes()
}

func handleCommands(w http.ResponseWriter, r *http.Request) {
	log.Debugln("handleCommands")
	commandLock.Lock()
	defer commandLock.Unlock()

	// get an ordered list of the command ids
	var ids []int
	for k, _ := range commands {
		ids = append(ids, k)
	}
	sort.Ints(ids)

	if len(ids) == 0 {
		resp := "<html>no commands founds</html>"
		w.Write([]byte(resp))
		return
	}

	// list the commands
	resp := "<html><table border=1><tr><td>Command ID</td><td>Type</td><td>Command</td><td>Files -> client</td><td>Files <- client</td><td>Log level</td><td>Log Path</td><td>Record Responses</td><td>Number of responses</td><td>Delete Command</td><td>Delete Command Response Files</td><td>Resubmit</td></tr>"

	for _, k := range ids {
		c := commands[k]
		deletePath := fmt.Sprintf("<a href=\"/command/delete?id=%v\">Delete Command</a>", c.ID)
		deleteFilesPath := fmt.Sprintf("<a href=\"/command/deletefiles?id=%v\">Delete Command Files</a>", c.ID)
		resubmitPath := fmt.Sprintf("<a href=\"/command/resubmit?id=%v\">Resubmit</a>", c.ID)
		resp += fmt.Sprintf("<tr><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td></tr>", c.ID, c.Type, c.Command, c.FilesSend, c.FilesRecv, c.LogLevel, c.LogPath, c.Record, len(c.checkedIn), deletePath, deleteFilesPath, resubmitPath)
	}

	resp += "</table></html>"

	w.Write([]byte(resp))
}

func handleNewCommand(w http.ResponseWriter, r *http.Request) {
	log.Debugln("handleNewCommand")

	// if no args, then present the new command dialog, otherwise try to parse the input
	commandType := r.FormValue("type")
	var resp string

	log.Debug("got type %v", commandType)

	switch commandType {
	case "exec":
		commandCmd := r.FormValue("command")
		if commandCmd == "" {
			resp = "<html>no command specified</html>"
		} else {
			commandFilesSend := r.FormValue("filesend")
			commandFilesRecv := r.FormValue("filerecv")
			commandRecord := r.FormValue("record")
			var record bool
			if commandRecord == "record" {
				record = true
			}
			c := &Command{
				Type:      COMMAND_EXEC,
				Record:    record,
				ID:        getCommandID(),
				Command:   strings.Fields(commandCmd),
				FilesSend: strings.Fields(commandFilesSend),
				FilesRecv: strings.Fields(commandFilesRecv),
			}
			log.Debug("generated command %v", c)
			commandLock.Lock()
			commands[c.ID] = c
			commandLock.Unlock()
			resp = fmt.Sprintf("<html>command %v submitted</html", c.ID)
		}
	case "filesend":
		commandFilesSend := r.FormValue("filesend")
		if commandFilesSend == "" {
			resp = "<html>no files specified</html>"
		} else {
			commandRecord := r.FormValue("record")
			var record bool
			if commandRecord == "record" {
				record = true
			}
			c := &Command{
				Type:      COMMAND_FILE_SEND,
				Record:    record,
				ID:        getCommandID(),
				FilesSend: strings.Fields(commandFilesSend),
			}
			log.Debug("generated command %v", c)
			commandLock.Lock()
			commands[c.ID] = c
			commandLock.Unlock()
			resp = fmt.Sprintf("<html>command %v submitted</html", c.ID)
		}
	case "filerecv":
		commandFilesRecv := r.FormValue("filerecv")
		if commandFilesRecv == "" {
			resp = "<html>no files specified</html>"
		} else {
			commandRecord := r.FormValue("record")
			var record bool
			if commandRecord == "record" {
				record = true
			}
			c := &Command{
				Type:      COMMAND_FILE_RECV,
				Record:    record,
				ID:        getCommandID(),
				FilesRecv: strings.Fields(commandFilesRecv),
			}
			log.Debug("generated command %v", c)
			commandLock.Lock()
			commands[c.ID] = c
			commandLock.Unlock()
			resp = fmt.Sprintf("<html>command %v submitted</html", c.ID)
		}
	case "log":
		commandLogLevel := r.FormValue("loglevel")
		if commandLogLevel == "" {
			resp = "<html>no log level specified</html>"
		} else {
			commandRecord := r.FormValue("record")
			var record bool
			if commandRecord == "record" {
				record = true
			}
			c := &Command{
				Type:     COMMAND_LOG,
				Record:   record,
				ID:       getCommandID(),
				LogLevel: commandLogLevel,
				LogPath:  r.FormValue("logpath"),
			}
			log.Debug("generated command %v", c)
			commandLock.Lock()
			commands[c.ID] = c
			commandLock.Unlock()
			resp = fmt.Sprintf("<html>command %v submitted</html", c.ID)
		}
	default:
		resp = `
			<html>
				<form method=post action=/command/new>
					Command type: <select name=type>
						<option selected value=exec>Execute</option>
						<option value=filesend>Send Files</option>
						<option value=filerecv>Receive Files</option>
						<option value=log>Chane log level</option>
					</select>
					<br>
					<input type=checkbox name=record value=record>Record Responses
					<br>
					Command: <input type=text name=command>
					<br>
					Files -> client (comma delimited) <input type=text name=filesend>
					<br>
					Files <- client (comma delimited) <input type=text name=filerecv>
					<br>
					New log level: <select name=loglevel>
						<option value=debug>Debug</option>
						<option value=info>Info</option>
						<option selected value=warn>Warn</option>
						<option value=error>Error</option>
						<option value=fatal>Fatal</option>
					</select>
					<br>
					Log file path: <input type=text name=logpath>
					<br>
					<input type=submit value=Submit>
				</form>
			</html>`
	}
	w.Write([]byte(resp))
}

func handleDeleteCommand(w http.ResponseWriter, r *http.Request) {
	log.Debugln("handleDeleteCommand")
	id := r.FormValue("id")
	val, err := strconv.Atoi(id)
	if err != nil {
		log.Errorln(err)
		w.Write([]byte(err.Error()))
		return
	}
	resp := commandDelete(val)
	resp = fmt.Sprintf("<html>%v</html>", resp)
	w.Write([]byte(resp))
}

func handleDeleteFiles(w http.ResponseWriter, r *http.Request) {
	log.Debugln("handleDeleteFiles")
	id := r.FormValue("id")
	val, err := strconv.Atoi(id)
	if err != nil {
		log.Errorln(err)
		w.Write([]byte(err.Error()))
		return
	}
	resp := commandDeleteFiles(val)
	resp = fmt.Sprintf("<html>%v</html>", resp)
	w.Write([]byte(resp))
}

func handleResubmit(w http.ResponseWriter, r *http.Request) {
	log.Debugln("handleResubmit")
	id := r.FormValue("id")
	val, err := strconv.Atoi(id)
	if err != nil {
		log.Errorln(err)
		w.Write([]byte(err.Error()))
		return
	}
	resp := commandResubmit(val)
	resp = fmt.Sprintf("<html>%v</html>", resp)
	w.Write([]byte(resp))
}

func updateCommands(newCommands map[int]*Command) {
	log.Debugln("updateCommands")
	commandLock.Lock()
	defer commandLock.Unlock()
	for k, v := range newCommands {
		if len(v.FilesSend) != 0 {
			//go deferUpdateCommand(v)
		} else {
			if w, ok := commands[k]; ok {
				v.checkedIn = w.checkedIn
			} else {
				log.Debug("new command %v", k)
			}
			commands[k] = v
		}
	}
}
