// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"bytes"
	"fmt"
	"html"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"mime/multipart"
	log "minilog"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
	"time"
)

// playbackServer is heavily based on http.FileServer. If the requested file is
// a directory, it will list the contents. Otherwise, it will try to stream
// JPEG images to the client that it reads from a recording.
type playbackServer struct {
	root http.FileSystem
}

func (s *playbackServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
		r.URL.Path = upath
	}

	serveFile(w, r, s.root, path.Clean(upath))
}

func serveFile(w http.ResponseWriter, r *http.Request, fs http.FileSystem, name string) {
	f, err := fs.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	d, err1 := f.Stat()
	if err1 != nil {
		http.NotFound(w, r)
		return
	}

	if d.IsDir() {
		// Directory, list the contents
		dirList(w, f)
	} else {
		// Actually playback a recording
		offset, err := time.ParseDuration(r.FormValue("offset"))
		if err != nil {
			log.Error("parse offset: %v: %v", err, r.FormValue("offset"))
			offset = 0
		}

		streamRecording(w, f, offset)
	}
}

// dirList writes an html page that provides links to all the files in a
// directory. This is copied from Go's net/http/fs.go file.
func dirList(w http.ResponseWriter, f http.File) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<pre>\n")
	for {
		dirs, err := f.Readdir(100)
		if err != nil || len(dirs) == 0 {
			break
		}

		for _, d := range dirs {
			name := d.Name()
			if d.IsDir() {
				name += "/"
			}
			// name may contain '?' or '#', which must be escaped to remain
			// part of the URL path, and not indicate the start of a query
			// string or fragment.
			url := url.URL{Path: name}
			fmt.Fprintf(w, "<a href=\"%s\">%s</a>\n", url.String(), html.EscapeString(name))
		}
	}
	fmt.Fprintf(w, "</pre>\n")
}

func streamRecording(w http.ResponseWriter, f http.File, start time.Duration) {
	updateChan, _ := readFile(f)
	imageChan := make(chan image.Image, 10)

	// gorountine to rebuild the images
	go func() {
		var X, Y int

		prev := time.Now()
		img := image.NewRGBA(image.Rect(0, 0, X, Y))
		var startOffset int64

		// for each jpeg image
		for update := range updateChan {
			var skip bool
			// fast forward to start
			// we set skip instead of just continuing the outer
			// loop so we can do other checks like resolution
			// changes
			if startOffset < start.Nanoseconds() {
				startOffset += update.Offset
				skip = true
			}

			// Check if the resolution has changed
			last := update.Rectangles[len(update.Rectangles)-1]
			if last.EncodingType == DesktopSize || X == 0 || Y == 0 {
				X = last.Rect.Max.X
				Y = last.Rect.Max.Y
			}

			nimg := image.NewRGBA(image.Rect(0, 0, X, Y))

			// Copy in the previous image
			dr := image.Rectangle{img.Rect.Min, img.Rect.Max}
			draw.Draw(nimg, dr, img, img.Rect.Min, draw.Src)

			for _, r := range update.Rectangles {
				dr := image.Rectangle{r.Rect.Min, r.Rect.Max}
				log.Debug("drawing in rectangle at %#v\n", dr)
				draw.Draw(nimg, dr, r, r.Rect.Min, draw.Src)
			}

			offset := time.Now().Sub(prev).Nanoseconds()
			img = nimg

			if skip {
				continue
			}

			prev = time.Now()

			if offset < update.Offset {
				// Sleep until the next image should be served
				time.Sleep(time.Duration(update.Offset - offset))
			} else {
				log.Debugln("warning: longer to replay images than record them")
			}

			imageChan <- nimg
		}

		close(imageChan)
	}()

	mh := make(textproto.MIMEHeader)
	mh.Set("Content-Type", "image/jpeg")

	m := multipart.NewWriter(w)
	defer m.Close()

	h := w.Header()
	boundary := m.Boundary()
	h.Set("Content-type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", boundary))

	// encode and send the image
	var buf bytes.Buffer
	for image := range imageChan {
		buf.Reset()

		log.Debug("writing image: %v", image.Bounds())
		err := jpeg.Encode(&buf, image, nil)
		if err != nil {
			log.Error("unable to encode jpeg: %v", err)
			break
		}

		mh.Set("Content-length", fmt.Sprintf("%d", buf.Len()))
		fm, err := m.CreatePart(mh)
		if err != nil {
			log.Error("unable to create multipart: %v", err)
			return
		}
		_, err = io.Copy(fm, &buf)
		if err != nil {
			log.Error("unable to write multipart: %v", err)
			break
		}
	}
}
