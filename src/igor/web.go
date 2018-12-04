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
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"ranges"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var cmdWeb = &Command{
	UsageLine: "web",
	Short:     "run a web application",
	Long: `
Run a Go web application with a GUI for igor -p for port

OPTIONAL FLAGS:

The -p flag sets the port of the server (default = 8080).

The -f flag sets location of html and static folder (default = current path).

The -s flag silences output.`,
}

// argument variables explained above
var webP string // port
var webF string // location of static folder
var webS bool   // silent
var webE string // path to igor executable

func init() {
	// break init cycle
	cmdWeb.Run = runWeb

	cmdWeb.Flag.StringVar(&webP, "p", "8080", "port")
	cmdWeb.Flag.StringVar(&webF, "f", "", "path to static resources")
	cmdWeb.Flag.BoolVar(&webS, "s", false, "silence output")
	cmdWeb.Flag.StringVar(&webE, "e", "igor", "path to igor executable")
}

// reservation object that igorweb.js understands
// an array of these is passed to client
// need to convert data to this structure in order to send it to client
type ResTableRow struct {
	Name  string
	Owner string
	// display string for "Start Time"
	Start string
	// integer start time for comparisons
	StartInt int64
	// display string for "End Time"
	End string
	// integer end time for comparisons
	EndInt int64
	// list of individual nodes in reservation
	// use RangeToInts for conversion from range
	Nodes []int
}

// object conataining a single option for speculate
// an array of ten of these is passed to the client
type Speculate struct {
	// display string for "Start Time" in speculate page
	Start string
	// display string for "End Time" in speculate page
	End string
	// properly formatted start string to be used in -a tag if Reserve is
	//              clicked in speculate page
	Formatted string
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

// updates reservation data and returns an array with the updated reservation info
// the first reservation (index 0) is all of the down nodes
//		and its StartInt is the current time
//			(for comparison in order to label current reservations)
func getReservations() []ResTableRow {

	// read data from files, update info, unlock files
	lock, _ := lockAndReadData(true)
	housekeeping()
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	resRows := []ResTableRow{}
	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)

	// resRows[0] => down nodes
	resRows = append(resRows, ResTableRow{
		"",
		"",
		"",
		time.Now().Unix(),
		"",
		0,
		rnge.RangeToInts(getDownNodes(getNodes())),
	})

	// convert all of the Reservations to ResTableRows
	timefmt := "Jan 2 15:04"
	for _, r := range Reservations {
		resRows = append(resRows, ResTableRow{
			r.ResName,
			r.Owner,
			time.Unix(r.StartTime, 0).Format(timefmt),
			r.StartTime,
			time.Unix(r.EndTime, 0).Format(timefmt),
			r.EndTime,
			rnge.RangeToInts(r.Hosts),
		})
	}
	return resRows
}

// Returns a non-nil error if something's wrong with the igor command
// and argument list. We expect that the first item in "args" is "igor"
func validCommand(args []string) error {
	// Check that the command starts with 'igor'
	if args[0] != "igor" {
		return errors.New("Not an igor command.")
	}

	// Check for valid subcommand
	found := false
	for _, c := range commands {
		if args[1] == c.Name() {
			found = true
			break
		}
	}
	if !found {
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

	// if an actual command (not heartbeat), run it and log response and error
	if splitcmd[1] != "show" {
		username, err := userFromAuthHeader(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		cmd := make([]string, len(splitcmd))
		copy(cmd, splitcmd)
		cmd[0] = webE

		env := []string{"USER=" + username}
		out, err = processWrapperEnv(env, cmd[0:]...)
		housekeeping()
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
		extra = getReservations()
	}

	// clean up response message
	re := regexp.MustCompile("\x1b\\[..?m")

	// create Response object
	rsp := Response{err == nil, fmt.Sprintln(re.ReplaceAllString(out, "")), extra}

	// write to output if not silent
	if !webS {
		fmt.Println("Command:", command)
		fmt.Println("\tFrom:", r.RemoteAddr)
		fmt.Println("\tResponse:", rsp.Message)
	}

	// send response
	jsonrsp, _ := json.Marshal(rsp)
	w.Write([]byte(jsonrsp))
}

// general handler for requests, only accepts requests to /
func handler(w http.ResponseWriter, r *http.Request) {
	if !webS {
		fmt.Println(r.Method, r.URL, r.RemoteAddr)
	}

	// serve igorweb.html with JS template variables filled in
	//              for initial display of reservation info
	if r.URL.Path == "/" {
		resRows := getReservations()
		t, err := template.ParseFiles(filepath.Join(webF, "igorweb.html"))
		if err != nil {
			panic(err)
		}
		data := struct {
			StartNode    int
			EndNode      int
			RackWidth    int
			Cluster      string
			ResTableRows []ResTableRow
		}{igorConfig.Start, igorConfig.End, igorConfig.Rackwidth, igorConfig.Prefix, resRows}

		err = t.Execute(w, data)
		if err != nil {
			panic(err)
		}
	} else {
		// reject all other requests
		http.Error(w, "404 not found.", http.StatusNotFound)
	}

}

// main web function
func runWeb(_ *Command, _ []string) {
	// handle requests for files in /static/
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(webF, "static")))))
	// general requests
	http.HandleFunc("/", handler)
	// commands
	http.HandleFunc("/run/", cmdHandler)
	// spin up server on specified port
	log.Fatal(http.ListenAndServe("127.0.0.1:"+webP, nil))
}
