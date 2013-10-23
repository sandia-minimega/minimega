package main

import (
	"bytes"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"io/ioutil"
	"math/rand"
	log "minilog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	MAX_CACHE = 128
)

var (
	htmlTemplate  *template.Template
	hits          uint
	hitChan       chan uint
	httpSiteCache []string
	httpImage     []byte
)

type HtmlContent struct {
	URLs   []string
	Hits   uint
	URI    string
	Secure bool
	Host   string
}

func httpClient() {
	log.Debugln("httpClient")

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	for {
		t.Tick()
		h, o := randomHost()
		log.Debug("http host %v from %v", h, o)
		httpClientRequest(h)
	}
}

func httpClientRequest(h string) {
	httpSiteCache = append(httpSiteCache, h)
	if len(httpSiteCache) > MAX_CACHE {
		httpSiteCache = httpSiteCache[len(httpSiteCache)-MAX_CACHE:]
	}

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	url := httpSiteCache[r.Int31()%int32(len(httpSiteCache))]

	log.Debugln("http using url: ", url)

	if !strings.HasPrefix(url, "http://") {
		url = "http://" + url
	}
	resp, err := http.Get(url)
	if err != nil {
		log.Errorln(err)
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	// make sure to grab any images, javascript, css
	extraFiles := parseBody(string(body))
	for _, v := range extraFiles {
		log.Debugln("grabbing extra file: ", v)
		httpGet(url, v)
	}

	links := parseLinks(string(body))
	if len(links) > 0 {
		httpSiteCache = append(httpSiteCache, links...)
		if len(httpSiteCache) > MAX_CACHE {
			httpSiteCache = httpSiteCache[len(httpSiteCache)-MAX_CACHE:]
		}
	}
}

func httpGet(url, file string) {
	if !strings.HasPrefix(file, "http://") {
		file = url + "/" + file
	}
	resp, err := http.Get(file)
	if err == nil {
		resp.Body.Close()
	}
}

func parseBody(body string) []string {
	var ret []string
	img := `src=[\'"]?([^\'" >]+)`

	images := regexp.MustCompile(img)
	i := images.FindAllStringSubmatch(body, -1)
	for _, v := range i {
		ret = append(ret, v[1])
	}

	log.Debugln("got extra files: ", ret)
	return ret
}

func parseLinks(body string) []string {
	var ret []string
	lnk := `href=[\'"]?([^\'" >]+)`

	links := regexp.MustCompile(lnk)
	i := links.FindAllStringSubmatch(body, -1)
	for _, v := range i {
		ret = append(ret, v[1])
	}

	log.Debugln("got links: ", ret)
	return ret
}

func httpServer() {
	http.HandleFunc("/", httpHandler)
	httpMakeImage()
	http.HandleFunc("/image.png", httpImageHandler)
	var err error
	htmlTemplate, err = template.New("output").Parse(htmlsrc)
	if err != nil {
		log.Fatalln(err)
	}
	hitChan = make(chan uint, 1024)
	go hitCounter()
	log.Fatalln(http.ListenAndServe(":80", nil))
}

func httpMakeImage() {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	m := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	for i := 0; i < 1024*768; i++ {
		m.Pix[i] = uint8(r.Int())
	}

	b := new(bytes.Buffer)
	png.Encode(b, m)
	httpImage = b.Bytes()
}

func hitCounter() {
	for {
		c := <-hitChan
		hits += c
	}
}

func httpImageHandler(w http.ResponseWriter, r *http.Request) {
	w.Write(httpImage)
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug("request: %v %v", r.RemoteAddr, r.URL.String())
	hitChan <- 1
	h := &HtmlContent{
		URLs: randomURLs(),
		Hits: hits,
		URI:  fmt.Sprintf("%v %v", r.RemoteAddr, r.URL.String()),
		Host: r.Host,
	}
	err := htmlTemplate.Execute(w, h)
	if err != nil {
		log.Errorln(err)
	}
}

func randomURLs() []string {
	var ret []string
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	for i := 0; i < 3; i++ {
		url := r.Int31()
		ret = append(ret, fmt.Sprintf("%v", url))

	}
	log.Debugln("random urls: ", ret)
	return ret
}

var htmlsrc = `
<h1>protonuke</h1>

<p>request URI: {{.URI}}</p>

<p>
{{range $v := .URLs}} 
<a href="http://{{$.Host}}/{{$v}}">{{$v}}</a><br>
{{end}}
</p>

<p>
hits: {{.Hits}}<br>
</p>

<p>
<img src=image.png>
</p>
`
