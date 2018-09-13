package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
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

var webP string // port
var webF string // location of static folder
var webS bool   // silent

func init() {
	// break init cycle
	cmdWeb.Run = runWeb

	cmdWeb.Flag.StringVar(&webP, "p", "8080", "")
	cmdWeb.Flag.StringVar(&webF, "f", "", "")
	cmdWeb.Flag.BoolVar(&webS, "s", false, "")
}

type ResTableRow struct {
	Name     string
	Owner    string
	Start    string
	StartInt int64
	End      string
	EndInt   int64
	Nodes    []int
}

type Speculate struct {
	Start     string
	End       string
	Formatted string
}

type Response struct {
	Success bool
	Message string
	Extra   interface{}
}

func throw404(w http.ResponseWriter) {
	http.Error(w, "404 not found.", http.StatusNotFound)
}

func getReservations() []ResTableRow {

	lock, _ := lockAndReadData(true)

	housekeeping()

	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	resRows := []ResTableRow{}
	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)

	resRows = append(resRows, ResTableRow{
		"",
		"",
		"",
		time.Now().Unix(),
		"",
		0,
		rnge.RangeToInts(getDownNodes(getNodes())),
	})

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

func cmdHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	command := r.URL.Query()["run"][0]
	splitcmd := strings.Split(command, " ")
	var extra interface{}
	log := ""
	var err error = nil
	if splitcmd[1] != "show" {
		log, err = processWrapper(splitcmd[0:]...)
		housekeeping()
	}
	if splitcmd[1] == "sub" && splitcmd[len(splitcmd)-1] == "-s" && err == nil {
		specs := []Speculate{}
		splitlog := strings.FieldsFunc(log, func(c rune) bool {
			return c == '\n' || c == '\t'
		})
		oldtimefmt := "2006-Jan-2-15:04"
		timefmt := "Jan 2 15:04"
		for i := 3; i < len(splitlog); i += 2 {
			t1, _ := time.Parse(oldtimefmt, splitlog[i])
			t2, _ := time.Parse(oldtimefmt, splitlog[i+1])
			specs = append(specs, Speculate{t1.Format(timefmt), t2.Format(timefmt), splitlog[i]})
		}
		extra = specs
	} else {
		extra = getReservations()
	}
	re := regexp.MustCompile("\x1b\\[..?m")
	rsp := Response{err == nil, fmt.Sprintln(re.ReplaceAllString(log, "")), extra}
	if !webS {
		fmt.Println("Command:", command)
		fmt.Println("\tFrom:", r.RemoteAddr)
		fmt.Println("\tResponse:", rsp.Message)
	}
	jsonrsp, _ := json.Marshal(rsp)
	w.Write([]byte(jsonrsp))
}

func handler(w http.ResponseWriter, r *http.Request) {

	if !webS {
		fmt.Println(r.Method, r.URL, r.RemoteAddr)
	}
	resRows := getReservations()

	if r.URL.Path == "/" {
		t, err := template.ParseFiles(webF + "igorweb.html")
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
		file := r.URL.Path[1:]
		dot := strings.Index(file, ".")
		if dot != -1 {
			ext := file[dot:]
			contents, err := ioutil.ReadFile(file)
			if err != nil {
				throw404(w)
				return
			}
			if ext == ".css" {
				w.Header().Add("Content-Type", "text/css")
				t, err := template.New("").Parse(
					fmt.Sprintf("%s", string(contents)),
				)
				err = t.Execute(w, nil)
				if err != nil {
					panic(err)
				}
			}
		}
	}

}

func runWeb(_ *Command, _ []string) {

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(webF+"static"))))
	http.HandleFunc("/", handler)
	http.HandleFunc("/run/", cmdHandler)
	log.Fatal(http.ListenAndServe(":"+webP, nil))
}
