// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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
		offset := time.Duration(0)
		formOffset := r.FormValue("offset")
		if formOffset != "" {
			offset, err = time.ParseDuration(formOffset)
			if err != nil {
				log.Error("parse offset: %v: %v", err, r.FormValue("offset"))
				offset = time.Duration(0)
			}
		}

		streamRecording(r.Context(), w, f, offset)
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

// streamRecording sends the recording in f as a stream of JPEG images, with
// the recorded delays between them. It skips the images before start. It
// returns when the recording ends, when the client disconnects, or when ctx
// is canceled, and it stops the reader of f before it returns.
func streamRecording(ctx context.Context, w http.ResponseWriter, f io.Reader, start time.Duration) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	updates, err := readFile(ctx, f)
	if err != nil {
		log.Error("unable to read recording: %v", err)
		http.Error(w, "unable to read recording", http.StatusInternalServerError)
		return
	}
	defer func() {
		cancel()
		for range updates {
		}
	}()

	mh := make(textproto.MIMEHeader)
	mh.Set("Content-Type", "image/jpeg")

	m := multipart.NewWriter(w)
	defer m.Close()

	h := w.Header()
	boundary := m.Boundary()
	h.Set("Content-type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", boundary))

	var (
		canvas = image.NewRGBA(image.Rectangle{})
		sentAt time.Duration // recording time of the last image sent
		next   time.Time     // wall time to send the next image
		buf    bytes.Buffer
	)

	for update := range updates {
		elapsed := update.Elapsed

		if len(update.Rectangles) == 0 {
			continue
		}

		canvas = applyUpdate(canvas, update)

		// fast forward to start
		if elapsed < start || canvas.Rect.Empty() {
			continue
		}

		if next.IsZero() {
			next = time.Now()
		} else {
			next = next.Add(elapsed - sentAt)
		}
		sentAt = elapsed

		if !sleepUntil(ctx, next) {
			return
		}

		buf.Reset()

		log.Debug("writing image: %v", canvas.Bounds())
		if err := jpeg.Encode(&buf, canvas, nil); err != nil {
			log.Error("unable to encode jpeg: %v", err)
			return
		}

		mh.Set("Content-length", fmt.Sprintf("%d", buf.Len()))
		fm, err := m.CreatePart(mh)
		if err != nil {
			log.Error("unable to create multipart: %v", err)
			return
		}
		if _, err := io.Copy(fm, &buf); err != nil {
			log.Error("unable to write multipart: %v", err)
			return
		}
	}
}

// applyUpdate draws the rectangles of u on canvas. A DesktopSize rectangle
// sets the framebuffer size. With no DesktopSize rectangle, the canvas grows
// to hold each rectangle. It returns the canvas, which is new when the size
// changes.
func applyUpdate(canvas *image.RGBA, u *FramebufferUpdate) *image.RGBA {
	for _, r := range u.Rectangles {
		if r.EncodingType == DesktopSize {
			canvas = resize(canvas, image.Rect(0, 0, r.Rect.Dx(), r.Rect.Dy()))
			continue
		}

		if !r.Rect.In(canvas.Rect) {
			b := image.Rect(0, 0, max(canvas.Rect.Max.X, r.Rect.Max.X), max(canvas.Rect.Max.Y, r.Rect.Max.Y))
			canvas = resize(canvas, b)
		}

		log.Debug("drawing in rectangle at %#v\n", r.Rect)
		draw.Draw(canvas, r.Rect, r, r.Rect.Min, draw.Src)
	}

	return canvas
}

// resize returns a canvas of bounds b with the pixels of canvas.
func resize(canvas *image.RGBA, b image.Rectangle) *image.RGBA {
	if b == canvas.Rect {
		return canvas
	}

	n := image.NewRGBA(b)
	draw.Draw(n, canvas.Rect, canvas, canvas.Rect.Min, draw.Src)
	return n
}

// sleepUntil waits until t. It returns false if ctx is canceled first.
func sleepUntil(ctx context.Context, t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err() == nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
