package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	log "minilog"
	"net/http"
	"sort"
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
	checkedIn []string
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
	commands       map[int]*Command
	commandCounter int
	commandLock    sync.Mutex
)

func init() {
	commands = make(map[int]*Command)
}

func getCommandID() int {
	log.Debugln("getCommandID")
	commandLock.Lock()
	defer commandLock.Unlock()
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
	commandLock.Lock()
	defer commandLock.Unlock()
	if id > commandCounter {
		log.Debug("found higher ID %v", id)
		commandCounter = id
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
	resp := "<html><table border=1><tr><td>Command ID</td><td>Type</td><td>Command</td><td>Files -> client</td><td>Files <- client</td><td>Log level</td><td>Log Path</td><td>Number of responses</td><td>Delete Command</td><td>Delete Command Response Files</td><td>Resubmit</td></tr>"

	for _, k := range ids {
		c := commands[k]
		deletePath := fmt.Sprintf("<a href=\"/command/delete?id=%v\">Delete Command</a>", c.ID)
		deleteFilesPath := fmt.Sprintf("<a href=\"/command/deletefiles?id=%v\">Delete Command Files</a>", c.ID)
		resubmitPath := fmt.Sprintf("<a href=\"/command/resubmit?id=%v\">Resubmit</a>", c.ID)
		resp += fmt.Sprintf("<tr><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td></tr>", c.ID, c.Type, c.Command, c.FilesSend, c.FilesRecv, c.LogLevel, c.LogPath, len(c.checkedIn), deletePath, deleteFilesPath, resubmitPath)
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
			c := &Command{
				Type:      COMMAND_EXEC,
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
			c := &Command{
				Type:      COMMAND_FILE_SEND,
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
			c := &Command{
				Type:      COMMAND_FILE_RECV,
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
			c := &Command{
				Type:     COMMAND_LOG,
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
	w.Write([]byte("not implemented"))
}

func handleDeleteFiles(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("not implemented"))
}

func handleResubmit(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("not implemented"))
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
