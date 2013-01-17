package novnctun

import (
	"encoding/base64"
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"
	"websocket"
)

const BUF = 32768

type Host_list interface {
	Hosts() map[string][]string
}

type Tun struct {
	Addr   string
	Hosts  Host_list
	Files  string // path to files for novnc to serve
	Unsafe bool
}

func (t *Tun) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// there are four things we can serve:
	// 	1. "/" - show the list of t.Hosts keys
	//	2. "/<host>" - show the list of t.Hosts[host] values
	//	3. "/<host>/<value>" - redirect to the novnc html with a path
	//	4. "/ws/<host>/<value>" - create a tunnel
	url := r.URL.String()
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	fields := strings.Split(url, "/")
	switch len(fields) {
	case 2: // "/"
		w.Write([]byte(t.html_hosts()))
	case 3: // "/<host>"
		w.Write([]byte(t.html_ports(fields[1])))
	case 4: // "/<host>/<port>"
		if t.allowed(fields[1], fields[2]) {
			title := html.EscapeString(fields[1] + ":" + fields[2])
			path := fmt.Sprintf("/novnc/vnc_auto.html?title=%v&path=ws/%v/%v", title, fields[1], fields[2])
			http.Redirect(w, r, path, http.StatusTemporaryRedirect)
		} else {
			http.NotFound(w, r)
		}
	case 5: // "/ws/<host>/<port>"
		if t.allowed(fields[2], fields[3]) {
			t.ws_handler(w, r)
		} else {
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

func (t *Tun) html_hosts() (body string) {
	hosts := t.Hosts.Hosts()
	if len(hosts) == 0 {
		body = "no hosts found"
		return
	}
	for i, v := range hosts {
		body += fmt.Sprintf("<a href=\"%v\">%v</a> (%v)<br>\n", i, i, len(v))
	}
	return
}

func (t *Tun) html_ports(host string) (body string) {
	ports := t.Hosts.Hosts()[host]
	if ports == nil {
		body = "no ports found for host: " + host
		return
	}
	for _, i := range ports {
		body += "<a href=\"/" + host + "/" + i + "\">" + i + "<br>\n"
	}
	return
}

func (t *Tun) Start() error {
	http.Handle("/", t)
	http.Handle("/novnc/", http.StripPrefix("/novnc/", http.FileServer(http.Dir(t.Files))))
	return http.ListenAndServe(t.Addr, nil)
}

func (t *Tun) allowed(host, port string) bool {
	if t.Unsafe {
		return true
	}
	l := t.Hosts.Hosts()
	h := l[host]
	if h == nil {
		return false
	}
	for _, i := range h {
		if i == port {
			return true
		}
	}
	return false
}

func (t *Tun) ws_handler(w http.ResponseWriter, r *http.Request) {
	// we assume that if we got here, then the url must be sane and of 
	// the format /<host>/<port>
	url := r.URL.String()
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	fields := strings.Split(url, "/")
	if len(fields) != 5 {
		http.NotFound(w, r)
		return
	}

	rhost := fmt.Sprintf("%v:%v", fields[2], fields[3])

	// connect to the remote host
	remote, err := net.Dial("tcp", rhost)
	if err != nil {
		http.StatusText(500)
		return
	}

	websocket.Handler(func(ws *websocket.Conn) {
		go func() {
			sbuf := make([]byte, BUF)
			dbuf := make([]byte, BUF)
			for {
				n, err := ws.Read(sbuf)
				if err != nil {
					break
				}
				n, err = base64.StdEncoding.Decode(dbuf, sbuf[0:n])
				if err != nil {
					break
				}
				_, err = remote.Write(dbuf[0:n])
				if err != nil {
					break
				}
			}
			remote.Close()
		}()
		func() {
			sbuf := make([]byte, BUF)
			dbuf := make([]byte, 2*BUF)
			for {
				n, err := remote.Read(sbuf)
				if err != nil {
					break
				}
				base64.StdEncoding.Encode(dbuf, sbuf[0:n])
				n = base64.StdEncoding.EncodedLen(n)
				_, err = ws.Write(dbuf[0:n])
				if err != nil {
					break
				}
			}
			ws.Close()
		}()
	}).ServeHTTP(w, r)
}
