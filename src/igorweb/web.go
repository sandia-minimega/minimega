/**********************************************
 * web.go
 * -----------
 * This file runs the web server for the igor web command.
 * First it serves the client igorweb.html, which references all of the files
 * in static/. Then, as the user executes commands, this program receives them at
 * [path-to-server]/run/[command], runs the commands on igor itself, and returns
 * the responses from igor the the client.
 *********************************************/

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	log "minilog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var help = `
Run a Go web application with a GUI for igor

OPTIONAL FLAGS:
`

func usage() {
	fmt.Fprintln(os.Stderr, help)
	flag.PrintDefaults()

	os.Exit(2)
}

var commands = map[string]bool{
	"del":    true,
	"show":   true,
	"stats":  true,
	"sub":    true,
	"power":  true,
	"extend": true,
	"notify": true,
	"sync":   true,
	"edit":   true,
}

var webP string // port
var webF string // location of static folder
var webS bool   // silent
var webE string // path to igor executable

func init() {
	flag.StringVar(&webP, "p", "8080", "port")
	flag.StringVar(&webF, "f", "", "path to static resources")
	flag.BoolVar(&webS, "s", false, "silence output")
	flag.StringVar(&webE, "e", "igor", "path to igor executable")
}

// object containing the response from web.go to client
type Response struct {
	Success bool
	// string displayed in response box
	Message string
	// additional information:
	//              if speculate command - array of Speculate objects
	//              else - updated reservations array
	Extra interface{}
}

var (
	igorLock      sync.Mutex
	showCacheLock sync.RWMutex
	showCache     *Show
)

// "run" igor show
func show() *Show {
	if showCache == nil || time.Now().Sub(showCache.LastUpdated) > 10*time.Second {
		updateShow()
	}

	showCacheLock.RLock()
	defer showCacheLock.RUnlock()

	return showCache
}

// run "show" and update cache
func updateShow() {
	showCacheLock.Lock()
	defer showCacheLock.Unlock()

	igorLock.Lock()
	defer igorLock.Unlock()

	log.Debug("Updating reservations")

	out, err := processWrapper(webE, "show", "-json")
	if err != nil {
		log.Warn("Error updating reservations")
		return
	}

	data := new(Show)
	if err := json.Unmarshal([]byte(out), data); err != nil {
		log.Warn("Error unmarshaling reservations: %v", err)
		return
	}

	data.LastUpdated = time.Now()
	showCache = data
}

// Returns a non-nil error if something's wrong with the igor command
// and argument list. We expect that the first item in "args" is "igor"
func validCommand(args []string) error {
	// Check that the command starts with 'igor'
	if args[0] != "igor" {
		return errors.New("Not an igor command.")
	}

	// Check for valid subcommand
	if !commands[args[1]] {
		return errors.New("Invalid igor subcommand.")
	}

	// A-OK
	return nil
}

// Grabs the user's username from the Authorization header. This
// header must exist in incoming requests.
func userFromAuthHeader(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("Invalid user.")
	}

	// strip off "Basic " and decode
	authInfo, err := base64.StdEncoding.DecodeString(authHeader[6:])
	if err != nil {
		return "", errors.New("Invalid user.")
	}

	// Remove :password if it's there
	return strings.Split(string(authInfo), ":")[0], nil
}

// handler for commands from client (sent through /run/[command])
//              "show" is run on heartbeat, no igor command needs to be run
func cmdHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	// separate command from path
	command := r.URL.Query()["run"][0]
	splitcmd := strings.Split(command, " ")

	var extra interface{} // for Response.Extra
	out := ""             // for Response.Message
	var err error = nil   // for Response.Success (if not nil)

	// Check that the igor command is valid
	if err := validCommand(splitcmd); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	username, err := userFromAuthHeader(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// if an actual command (not heartbeat), run it and log response and error
	if splitcmd[1] != "show" {
		cmd := make([]string, len(splitcmd))
		copy(cmd, splitcmd)
		cmd[0] = webE

		igorLock.Lock()

		env := []string{"USER=" + username}
		out, err = processWrapperEnv(env, cmd[0:]...)
		log.Debug(out)

		igorLock.Unlock()

		updateShow()
	}

	// if a speculate command
	if splitcmd[1] == "sub" && splitcmd[len(splitcmd)-1] == "-s" && err == nil {
		specs := []Speculate{}

		// parse response from igor
		splitlog := strings.FieldsFunc(out, func(c rune) bool {
			return c == '\n' || c == '\t'
		})

		// convert response to array of Speculates to pass in Response.Extra
		oldtimefmt := "2006-Jan-2-15:04"
		timefmt := "Jan 2 15:04"
		for i := 3; i < len(splitlog); i += 2 {
			t1, _ := time.Parse(oldtimefmt, splitlog[i])
			t2, _ := time.Parse(oldtimefmt, splitlog[i+1])
			specs = append(specs, Speculate{t1.Format(timefmt), t2.Format(timefmt), splitlog[i]})
		}
		extra = specs

	} else {
		// all other commands get an updated reservations array in Response.Extra
		extra = show().ResTable()
	}

	// clean up response message
	re := regexp.MustCompile("\x1b\\[..?m")

	// create Response object
	rsp := Response{err == nil, fmt.Sprintln(re.ReplaceAllString(out, "")), extra}

	// write to output if not silent
	if !webS {
		m := fmt.Sprintf("From: %s Command: %q \tResponse: %q", r.RemoteAddr, command, rsp.Message)
		log.Debug(m)
	}

	// send response
	jsonrsp, _ := json.Marshal(rsp)
	w.Write([]byte(jsonrsp))
}

// general handler for requests, only accepts requests to /
func handler(w http.ResponseWriter, r *http.Request) {
	if !webS {
		log.Debug(fmt.Sprintf("%s %s %s", r.Method, r.URL, r.RemoteAddr))
	}

	// serve igorweb.html with JS template variables filled in
	//              for initial display of reservation info
	if r.URL.Path == "/" {
		t, err := template.ParseFiles(filepath.Join(webF, "igorweb.html"))
		if err != nil {
			panic(err)
		}

		err = t.Execute(w, show())
		if err != nil {
			panic(err)
		}
	} else {
		// reject all other requests
		http.Error(w, "404 not found.", http.StatusNotFound)
	}

}

// main web function
func main() {
	flag.Usage = usage
	flag.Parse()

	log.Init()

	args := flag.Args()
	if len(args) > 0 {
		usage()
	}

	// handle requests for files in /static/
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(webF, "static")))))
	// general requests
	http.HandleFunc("/", handler)
	// commands
	http.HandleFunc("/run/", cmdHandler)
	// spin up server on specified port
	log.Fatal(http.ListenAndServe("127.0.0.1:"+webP, nil).Error())
}
