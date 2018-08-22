package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"ranges"
	"strings"
	"time"
)

var cmdWeb = &Command{
	UsageLine: "web",
	Short:     "run a web application",
	Long:      `Run a Go web application with a GUI for igor`,
}

func init() {
	// break init cycle
	cmdWeb.Run = runWeb
}

type ResTableRow struct {
	Name  string
	Owner string
	Start string
	End   string
	Nodes []int
}

type Speculate struct {
	Start string
	End   string
}

type Response struct {
	Success bool
	Message string
	Extra   interface{}
}

func throw404(w http.ResponseWriter) {
	http.Error(w, "404 not found.", http.StatusNotFound)
}

func cmdHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	var extra interface{}
	extra = nil
	command := r.URL.Query()["run"][0]
	fmt.Println("Command:", command)
	fmt.Println("\tFrom:", r.RemoteAddr)
	q := strings.Split(command, " ")
	var a Response
	if q[0] != "igor" {
		a = Response{false, "command must begin with 'igor'", extra}
	} else {
		switch q[1] {
		case "del":
			a = Response{true, "Delete! " + command, extra}
		case "sub":
			if q[len(q)-1] == "-s" {
				extra = []Speculate{
					Speculate{"Apr 25 09:37", "Apr 24 09:37"},
					Speculate{"Apr 26 09:37", "Apr 23 09:37"},
					Speculate{"Apr 27 09:37", "Apr 22 09:37"},
					Speculate{"Apr 28 09:37", "Apr 21 09:37"},
					Speculate{"Apr 29 09:37", "Apr 20 09:37"},
					Speculate{"Apr 30 09:37", "Apr 19 09:37"},
					Speculate{"Apr 31 09:37", "Apr 18 09:37"},
					Speculate{"Apr 32 09:37", "Apr 17 09:37"},
					Speculate{"Apr 33 09:37", "Apr 16 09:37"},
					Speculate{"Apr 34 09:37", "Apr 15 09:37"},
				}
			}
			a = Response{true, "Reserve! " + command, extra}
		case "power":
			a = Response{true, "Power! " + command, extra}
		case "extend":
			a = Response{true, "Extend! " + command, extra}
		default:
			a = Response{false, "command not supported (del, sub, power, extend)", extra}
		}
	}
	fmt.Println("\tResponse:", a.Message)
	s, _ := json.Marshal(a)
	w.Write([]byte(s))
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.Method, r.URL, r.RemoteAddr)

	resRows := []ResTableRow{}

	resRows = append(resRows, ResTableRow{
		"",
		"",
		"",
		"",
		ranges.RangeToInts(getDownNodes(getNodes())),
	})

	timefmt := "Jan 2 15:04"
	for _, r := range Reservations {
		resRows = append(resRows, ResTableRow{
			r.ResName,
			r.Owner,
			time.Unix(r.StartTime, 0).Format(timefmt),
			time.Unix(r.EndTime, 0).Format(timefmt),
			ranges.RangeToInts(r.Hosts),
		})
	}

	if r.URL.Path == "/" {
		t, err := template.ParseFiles("igorweb.html")
		if err != nil {
			panic(err)
		}
		data := struct {
			NumNodeCols  int
			NumNodes     int
			Cluster      string
			ResTableRows []ResTableRow
		}{16, 288, igorConfig.Prefix, resRows}

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

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/", handler)
	http.HandleFunc("/run/", cmdHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
