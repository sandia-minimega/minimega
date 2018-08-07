package main

import (
	"log"
	"net/http"
	"html/template"
	"fmt"
	"strings"
	"io/ioutil"
	"encoding/json"
)

type Reservation struct {
	Name string
	Owner string
	Start string
	End string
	Nodes []int
}

type Speculate struct {
	Start string
	End string
}

type Response struct {
	Success bool
	Message string
	Extra interface{}
}

func throw404(w http.ResponseWriter) {
	http.Error(w, "404 not found.", http.StatusNotFound)
}

func cmdHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println(w, r)
	w.Header().Set("Content-Type", "text/plain")
	var extra interface{}
	command := r.URL.Query()["run"][0]
	q := strings.Split(command, " ")
	if q[1] == "sub" && q[len(q) - 1] == "-s" {
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
	} else {
		extra = nil
	}
	a := Response{true, "Server responding to: " + command, extra}
	s, _ := json.Marshal(a)
	w.Write([]byte(s))
	// fmt.Fprintf(w, "Server successfully received this command: " + r.URL.Query()["run"][0])
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Println(w, r)
	if r.URL.Path == "/" {
		t, err := template.ParseFiles("igorweb.html")
		if err != nil {
			panic(err)
		}
		data := struct {
			NumNodeCols int
			NumNodes int
			Cluster string
			Reservations []Reservation
		}{16, 288, "kn",
			[]Reservation{
				Reservation{"", "", "", "", []int{54, 55, 265, 266}},
				Reservation{"jacob", "ryan", "Apr 25 09:37", "Apr 30 09:37", []int{1, 2, 3}},
				Reservation{"tis", "terrys", "Apr 25 09:37", "Apr 30 09:37", []int{165, 256, 260, 264}},
				Reservation{"ptare", "actualhuman", "Apr 25 09:37", "Apr 30 09:37", []int{39, 62, 84, 104, 106, 111, 124, 133, 138, 156, 163, 170, 204}},
				Reservation{"rcore", "throjs", "Apr 25 09:37", "Apr 30 09:37", []int{45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60}},
				Reservation{"program", "expertman", "Apr 25 09:37", "Apr 30 09:37", []int{288, 287, 286, 285, 284, 283, 282, 281, 280}},
		}}

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
				// t, err := template.ParseFiles("index.html")
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

func main() {

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/", handler)
	http.HandleFunc("/run/", cmdHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
