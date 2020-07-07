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
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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
var webD string // path to default kernel/init pairs

func init() {
	flag.StringVar(&webP, "p", "8080", "port")
	flag.StringVar(&webF, "f", "", "path to static resources")
	flag.BoolVar(&webS, "s", false, "silence output")
	flag.StringVar(&webE, "e", "igor", "path to igor executable")
	flag.StringVar(&webD, "d", "", "path to default kernel/init pairs")
}

// Response contains a response sent from the server to the client
type Response struct {
	Success bool

	// string displayed in response box
	Message string

	// additional information:
	//              if speculate command - array of Speculate objects
	//              else - updated reservations array
	Extra interface{}
}

// kiPair represents a kernel/init pair found in webD
type kiPair struct {
	Name   string
	Kernel string
	Initrd string
	valid  bool
}

var (
	// igorLock is locked if igor is being run by igorweb
	igorLock sync.Mutex

	// showCache stores cached results from the last time igorweb
	// ran "igor show"
	showCache *Show

	// showCacheLock is locked if showCache is being read or
	// written
	showCacheLock sync.RWMutex
)

// show fetches "igor show" data relevant to the given user. It
// updates the "igor show" cache if the cache is more than 10 seconds
// old.
func show(username string) *UserShow {
	if showCache == nil || time.Now().Sub(showCache.LastUpdated) > 10*time.Second {
		updateShow()
	}

	showCacheLock.RLock()
	defer showCacheLock.RUnlock()

	return &UserShow{showCache, username}
}

// updateShow runs "igor show" in a subprocess and updates the cache
func updateShow() {
	log.Debug("Running update Show")
	showCacheLock.Lock()
	defer showCacheLock.Unlock()

	igorLock.Lock()
	defer igorLock.Unlock()

	log.Debug("Updating reservations cache")

	out, err := processWrapper(webE, "show", "-json")
	if err != nil {
		log.Warn("Error updating reservations cache")

		// Strip out ANSI colors
		msg := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(out, "")

		// Strip off log header
		msg = regexp.MustCompile("^.*\\.go:[0-9]+:").ReplaceAllString(msg, "")

		showCache = &Show{
			Error:       msg,
			LastUpdated: time.Now(),
		}
		return
	}

	data := new(Show)
	if err := json.Unmarshal([]byte(out), data); err != nil {
		log.Warn("Error unmarshaling reservations: %v", err)
		return
	}

	data.LastUpdated = time.Now()
	data.Listimages = getDefaultImages()
	data.Path = webD
	showCache = data
}

// validCommand returns a non-nil error if something's wrong with the
// igor command and argument list. We expect that the first item in
// "args" is "igor"
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

// userFromAuthHeader grabs the user's username from the Authorization
// header. This header must exist in incoming requests.
func userFromAuthHeader(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("Invalid user.")
	}

	// strip off "Basic " and decode
	authInfo, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
	if err != nil {
		return "", errors.New("Invalid user.")
	}

	// Remove :password if it's there
	return strings.Split(string(authInfo), ":")[0], nil
}

// cmdHandler handles commands from clients (sent through
// /run/[command]) "show" is run on heartbepat, no igor command needs
// to be run
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

	// Get the username from the auth header
	username, err := userFromAuthHeader(r)
	if err != nil {
		// Kick them out if we can't find the username
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
		extra = show(username).ResTable()
	}

	// clean up response message
	re := regexp.MustCompile("\x1b\\[..?m")

	// create Response object
	rsp := Response{err == nil, fmt.Sprintln(re.ReplaceAllString(out, "")), extra}

	// write to output if not silent
	if !webS {
		m := fmt.Sprintf("From: %s By: %s Command: %q \tResponse: %q", r.RemoteAddr, username, command, rsp.Message)

		if splitcmd[1] != "show" {
			log.Info(m)
		} else {
			log.Debug(m)
		}
	}

	// send response
	jsonrsp, _ := json.Marshal(rsp)
	w.Write([]byte(jsonrsp))
}

// handler handles all other requests. Kicks back 404 if Path is
// anything but /
func handler(w http.ResponseWriter, r *http.Request) {
	if !webS {
		log.Debug(fmt.Sprintf("%s %s %s", r.Method, r.URL, r.RemoteAddr))
	}

	// Determine the user's username
	username, err := userFromAuthHeader(r)
	if err != nil {
		// Kick them out if we can't find it
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// serve igorweb.html with JS template variables filled in
	//              for initial display of reservation info
	if r.URL.Path == "/" {
		t, err := template.ParseFiles(filepath.Join(webF, "igorweb.html"))
		if err != nil {
			panic(err)
		}

		err = t.Execute(w, show(username))
		if err != nil {
			panic(err)
		}

		log.Info("Initial page load by %s", username)
	} else {
		// reject all other requests
		http.Error(w, "404 not found.", http.StatusNotFound)
	}

}

// getDefaultImages grabs a list of kernel inits to serve to reservation drop down.
func getDefaultImages() map[string]*kiPair {
	log.Debugln("Getting Images")
	imagelist := make(map[string]*kiPair)
	if webD == "" {
		return imagelist
	}
	files, err := ioutil.ReadDir(webD)
	if err != nil {
		log.Warn("Error with Default images Path : %v ", err.Error())
		return imagelist
	}

	for _, file := range files {
		fn := strings.Split(file.Name(), ".")
		if len(fn) < 2 {
			continue
		}
		k := ""
		i := ""
		iskernel := false
		if fn[1] == "kernel" {
			iskernel = true
			k = file.Name()
		} else if fn[1] == "initrd" {
			i = file.Name()
		} else {
			continue
		}

		if pair, ok := imagelist[fn[0]]; ok {
			if iskernel {
				pair.Kernel = k
				if pair.Initrd != "" && pair.Kernel != "" {
					log.Debug("Found K/I Pair: %v", pair.Name)
					pair.valid = true
				}
			} else {
				pair.Initrd = i
				if pair.Initrd != "" && pair.Kernel != "" {
					log.Debug("Found K/I Pair: %v", pair.Name)
					pair.valid = true
				}
			}
		} else {
			log.Debugln("Creating new Pair")
			imagelist[fn[0]] = &kiPair{
				Name:   fn[0],
				Kernel: k,
				Initrd: i,
				valid:  false,
			}
		}
	}
	log.Debug("found %v pairs", len(imagelist))
	return imagelist
}

func main() {
	flag.Usage = usage
	flag.Parse()

	log.Init()

	args := flag.Args()
	if len(args) > 0 {
		usage()
	}

	// Kill the server if SUDO_USER is set. It interferes with subprocess calls to "igor"
	for _, envvar := range os.Environ() {
		if strings.HasPrefix(envvar, "SUDO_USER=") {
			log.Fatalln("SUDO_USER is set. This will likely cause problems with names on reservations.")
		}
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
