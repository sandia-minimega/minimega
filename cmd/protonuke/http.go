// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/version"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	MAX_CACHE = 128
)

var (
	htmlTemplate     *template.Template
	hits             uint64
	hitsTLS          uint64
	hitChan          chan uint64
	hitTLSChan       chan uint64
	httpSiteCache    []string
	httpTLSSiteCache []string
	httpReady        bool
	httpLock         sync.Mutex
	httpFS           http.Handler

	httpImageMu sync.RWMutex // guards below
	httpImages  map[FileSize][]byte
)

type HtmlContent struct {
	URLs   []string
	Hits   uint64
	URI    string
	Secure bool
	Host   string
}

func httpClient(protocol string) {
	log.Debugln("httpClient")

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: func(network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return dialer.Dial(protocol, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		// TODO: max client read timeouts configurable?
		//Timeout:   30 * time.Second,
	}

	if *f_httpCookies {
		// TODO: see note about PublicSuffixList in cookiejar.Options
		client.Jar, _ = cookiejar.New(nil)
	}

	for {
		t.Tick()
		h, o := randomHost()
		log.Debug("http host %v from %v", h, o)
		httpClientRequest(h, client)
		httpReportChan <- 1
	}
}

func httpTLSClient(protocol string) {
	log.Debugln("httpTLSClient")

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		Proxy:           http.ProxyFromEnvironment,
		Dial: func(network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return dialer.Dial(protocol, addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if *f_tlsVersion != "" {
		var version uint16
		switch *f_tlsVersion {
		case "tls1.0":
			version = tls.VersionTLS10
		case "tls1.1":
			version = tls.VersionTLS11
		case "tls1.2":
			version = tls.VersionTLS12
		}
		transport.TLSClientConfig.MinVersion = tls.VersionSSL30
		transport.TLSClientConfig.MaxVersion = version
	}

	client := &http.Client{
		Transport: transport,
		// TODO: max client read timeouts configurable?
		//Timeout:   30 * time.Second,
	}

	if *f_httpCookies {
		// TODO: see note about PublicSuffixList in cookiejar.Options
		client.Jar, _ = cookiejar.New(nil)
	}

	for {
		t.Tick()
		h, o := randomHost()
		log.Debug("https host %v from %v", h, o)
		httpTLSClientRequest(h, client)
		httpTLSReportChan <- 1
	}
}

func httpClientRequest(h string, client *http.Client) (elapsed uint64) {
	httpSiteCache = append(httpSiteCache, h)
	if len(httpSiteCache) > MAX_CACHE {
		httpSiteCache = httpSiteCache[len(httpSiteCache)-MAX_CACHE:]
	}

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	url := httpSiteCache[r.Int31()%int32(len(httpSiteCache))]

	log.Debugln("http using url: ", url)

	// url notation requires leading and trailing [] on ipv6 addresses
	if isIPv6(url) {
		url = "[" + url + "]"
	}

	if !strings.HasPrefix(url, "http://") {
		url = "http://" + url
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Errorln(err)
		return
	}

	if *f_httpUserAgent != "" {
		req.Header.Set("User-Agent", *f_httpUserAgent)
	}

	start := time.Now().UnixNano()

	resp, err := client.Do(req)
	if err != nil {
		log.Errorln(err)
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	stop := time.Now().UnixNano()
	elapsed = uint64(stop - start)
	log.Info("http %v %v %vns", h, url, elapsed)

	// make sure to grab any images, javascript, css
	extraFiles := parseBody(string(body))
	for _, v := range extraFiles {
		log.Debugln("grabbing extra file: ", v)
		httpGet(url, v, false, client)
	}

	links := parseLinks(string(body))
	if len(links) > 0 {
		httpSiteCache = append(httpSiteCache, links...)
		if len(httpSiteCache) > MAX_CACHE {
			httpSiteCache = httpSiteCache[len(httpSiteCache)-MAX_CACHE:]
		}
	}

	return
}

func httpTLSClientRequest(h string, client *http.Client) (elapsed uint64) {
	httpTLSSiteCache = append(httpTLSSiteCache, h)
	if len(httpTLSSiteCache) > MAX_CACHE {
		httpTLSSiteCache = httpTLSSiteCache[len(httpTLSSiteCache)-MAX_CACHE:]
	}

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	url := httpTLSSiteCache[r.Int31()%int32(len(httpTLSSiteCache))]

	log.Debugln("https using url: ", url)

	// url notation requires leading and trailing [] on ipv6 addresses
	if isIPv6(url) {
		url = "[" + url + "]"
	}

	if !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Errorln(err)
		return
	}

	if *f_httpUserAgent != "" {
		req.Header.Set("User-Agent", *f_httpUserAgent)
	}

	start := time.Now().UnixNano()
	resp, err := client.Do(req)
	if err != nil {
		log.Errorln(err)
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	stop := time.Now().UnixNano()
	elapsed = uint64(stop - start)
	log.Info("https %v %v %vns", client, url, elapsed)

	// make sure to grab any images, javascript, css
	extraFiles := parseBody(string(body))
	for _, v := range extraFiles {
		log.Debugln("grabbing extra file: ", v)
		httpGet(url, v, true, client)
	}

	links := parseLinks(string(body))
	if len(links) > 0 {
		httpTLSSiteCache = append(httpTLSSiteCache, links...)
		if len(httpTLSSiteCache) > MAX_CACHE {
			httpTLSSiteCache = httpTLSSiteCache[len(httpTLSSiteCache)-MAX_CACHE:]
		}
	}

	return
}

func httpGet(url, file string, useTLS bool, client *http.Client) {
	// url notation requires leading and trailing [] on ipv6 addresses
	if isIPv6(url) {
		url = "[" + url + "]"
	}

	if useTLS {
		if !strings.HasPrefix(file, "https://") {
			file = url + "/" + file
		}
		start := time.Now().UnixNano()
		resp, err := client.Get(file)
		if err != nil {
			log.Errorln(err)
		} else {
			n, err := io.Copy(ioutil.Discard, resp.Body)
			if err != nil {
				log.Error("httpGet: %v, only copied %v bytes", err, n)
			}
			resp.Body.Close()
			stop := time.Now().UnixNano()
			log.Info("https %v %v %vns", client, file, stop-start)
			//httpTLSReportChan <- 1
		}
	} else {
		if !strings.HasPrefix(file, "http://") {
			file = url + "/" + file
		}
		start := time.Now().UnixNano()
		resp, err := client.Get(file)
		if err != nil {
			log.Errorln(err)
		} else {
			n, err := io.Copy(ioutil.Discard, resp.Body)
			if err != nil {
				log.Error("httpGet: %v, only copied %v bytes", err, n)
			}
			resp.Body.Close()
			stop := time.Now().UnixNano()
			log.Info("http %v %v %vns", client, file, stop-start)
			//httpReportChan <- 1
		}
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

func httpSetup() {
	httpLock.Lock()
	defer httpLock.Unlock()

	if httpReady {
		return
	}
	httpReady = true

	if *f_httproot != "" {
		httpFS = http.FileServer(http.Dir(*f_httproot))
	}

	http.HandleFunc("/", httpHandler)

	httpImages = make(map[FileSize][]byte)
	httpMakeImage(f_httpImageSize)

	var err error
	htmlTemplate, err = template.New("output").Parse(htmlsrc)
	if err != nil {
		log.Fatalln(err)
	}
}

func httpServer(p string) {
	log.Debugln("httpServer")
	httpSetup()
	hitChan = make(chan uint64, 1024)
	go hitCounter()
	server := &http.Server{
		Addr:    ":http",
		Handler: nil,
	}
	server.SetKeepAlivesEnabled(false)

	conn, err := net.Listen(p, ":http")
	if err != nil {
		log.Fatalln(err)
	}

	log.Fatalln(server.Serve(conn))
}

func httpTLSServer(p string) {
	log.Debugln("httpTLSServer")
	httpSetup()
	hitTLSChan = make(chan uint64, 1024)
	go hitTLSCounter()
	cert, key := generateCerts()

	//log.Fatalln(http.ListenAndServeTLS(":https", cert, key, nil))
	server := &http.Server{
		Addr:    ":https",
		Handler: nil,
	}
	server.SetKeepAlivesEnabled(false)

	config := &tls.Config{}
	if config.NextProtos == nil {
		config.NextProtos = []string{"http/1.1"}
	}
	config.MinVersion = tls.VersionTLS10
	config.MaxVersion = tls.VersionTLS12

	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(cert, key)
	if err != nil {
		log.Fatalln(err)
	}

	conn, err := net.Listen(p, ":https")
	if err != nil {
		log.Fatalln(err)
	}

	tlsListener := tls.NewListener(conn, config)
	log.Fatalln(server.Serve(tlsListener))
}

func httpMakeImage(size FileSize) {
	pixelcount := size / 4
	side := int(math.Sqrt(float64(pixelcount)))
	log.Debug("Image served will be %v by %v", side, side)

	m := image.NewRGBA(image.Rect(0, 0, side, side))
	for i := 0; i < len(m.Pix); i++ {
		m.Pix[i] = uint8(rand.Int())
	}

	buf := new(bytes.Buffer)

	if *f_httpGzip {
		w := gzip.NewWriter(buf)
		png.Encode(w, m)
		w.Close()
	} else {
		png.Encode(buf, m)
	}

	httpImageMu.Lock()
	defer httpImageMu.Unlock()

	httpImages[size] = buf.Bytes()
}

func hitCounter() {
	for {
		c := <-hitChan
		hits++
		httpReportChan <- c
	}
}

func hitTLSCounter() {
	for {
		c := <-hitTLSChan
		hitsTLS++
		httpTLSReportChan <- c
	}
}

func httpImageHandler(w http.ResponseWriter, r *http.Request) {
	size := f_httpImageSize

	v := r.URL.Query()
	if v.Get("size") != "" {
		size2, err := ParseFileSize(v.Get("size"))
		if err != nil {
			http.Error(w, "invalid size", http.StatusBadRequest)
			return
		}
		size = size2
	}

	var buf []byte
	var ok bool

	httpImageMu.RLock()
	buf, ok = httpImages[size]
	httpImageMu.RUnlock()

	if !ok {
		httpMakeImage(size)
	}

	httpImageMu.RLock()
	buf, _ = httpImages[size]
	httpImageMu.RUnlock()

	if *f_httpGzip {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "image/png")
	}
	w.Write(buf)
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug("request: %v %v", r.RemoteAddr, r.URL.String())
	var usingTLS bool
	if r.TLS != nil {
		log.Debugln("request using tls")
		usingTLS = true
	}

	w.Header().Set("Server", "protonuke/"+version.Revision)

	start := time.Now().UnixNano()
	if httpFS != nil {
		httpFS.ServeHTTP(w, r)
	} else if r.Method == http.MethodPost {
		w.WriteHeader(http.StatusAccepted)
	} else if strings.HasSuffix(r.URL.Path, "image.png") {
		httpImageHandler(w, r)
	} else {
		h := &HtmlContent{
			URLs:   randomURLs(),
			Hits:   hits,
			URI:    fmt.Sprintf("%v %v", r.RemoteAddr, r.URL.String()),
			Host:   r.Host,
			Secure: usingTLS,
		}
		err := htmlTemplate.Execute(w, h)
		if err != nil {
			log.Errorln(err)
		}
	}

	stop := time.Now().UnixNano()
	elapsed := uint64(stop - start)

	if usingTLS {
		log.Info("https %v %v %vns", r.RemoteAddr, r.URL, elapsed)
		hitTLSChan <- 1
	} else {
		log.Info("http %v %v %vns", r.RemoteAddr, r.URL, elapsed)
		hitChan <- 1
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
<a href="http{{if $.Secure}}s{{end}}://{{$.Host}}/{{$v}}">{{$v}}</a><br>
{{end}}
</p>

<p>
hits: {{.Hits}}<br>
</p>

<p>
<img src=image.png>
</p>
`
