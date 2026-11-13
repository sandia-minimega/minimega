// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type Rectangle struct {
	*image.RGBA
	EncodingType int32
}

type FramebufferUpdate struct {
	// The recording time of the first byte of the update
	Elapsed time.Duration
	// The rectangles that should be updated
	Rectangles []Rectangle
}

type PixelFilter int

const (
	CopyFilter PixelFilter = iota
	PaletteFilter
	GradientFilter
)

type vncPixelFormat struct {
	BitsPerPixel, Depth, BigEndianFlag, TrueColorFlag uint8
	RedMax, GreenMax, BlueMax                         uint16
	RedShift, GreenShift, BlueShift                   uint8
	Padding                                           [3]byte
}

var (
	f_port = flag.Int("port", 9004, "port to start rfbplay webservice")
)

var pixelFormat = vncPixelFormat{
	BitsPerPixel:  0x20,
	Depth:         0x18,
	BigEndianFlag: 0x0,
	TrueColorFlag: 0x1,
	RedMax:        0xff,
	GreenMax:      0xff,
	BlueMax:       0xff,
	RedShift:      0x10,
	GreenShift:    0x8,
	BlueShift:     0x0,
}

func usage() {
	fmt.Println("Usage:")
	fmt.Println("\trfbplay [OPTION] <input> <output>")
	fmt.Println("\trfbplay [OPTION] <directory>")
	flag.PrintDefaults()
}

// readFile decodes the gzip recording in f and sends each update on the
// returned channel. It closes the channel at the end of the recording, on an
// error, or when ctx is canceled. It does not close f.
func readFile(ctx context.Context, f io.Reader) (<-chan *FramebufferUpdate, error) {
	gzipReader, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	reader := NewRecordingReader(bufio.NewReader(gzipReader))
	output := make(chan *FramebufferUpdate)

	go func() {
		defer close(output)

		tight := &tightDecoder{}
		for {
			update, err := readUpdate(reader, tight)
			if err != nil {
				if err != io.EOF {
					log.Errorln("error decoding recording:", err)
				}
				return
			}

			select {
			case output <- update:
			case <-ctx.Done():
				return
			}
		}
	}()

	return output, nil
}

func readUpdate(reader *RecordingReader, tight *tightDecoder) (*FramebufferUpdate, error) {
	// Decode the message type
	var mType uint8
	if err := binary.Read(reader, binary.BigEndian, &mType); err != nil {
		if err == io.EOF {
			return nil, err // No more updates
		}
		return nil, errors.New("unable to decode message type")
	}

	log.Debugln("message type:", mType)

	// Skip message that aren't framebuffer updates
	if mType != 0 {
		return nil, errors.New("unable to decode, found non-framebuffer update")
	}

	update := &FramebufferUpdate{Elapsed: reader.Elapsed()}

	// Skip the one byte of padding
	if _, err := io.ReadFull(reader, make([]byte, 1)); err != nil {
		return nil, errors.New("unable to skip padding")
	}

	// Decode the number of rectangles
	var numRects uint16
	if err := binary.Read(reader, binary.BigEndian, &numRects); err != nil {
		return nil, errors.New("unable to decode number of rectangles")
	}

	log.Debugln("number of rectangles:", numRects)

	// Read all the rectangles
	for len(update.Rectangles) < int(numRects) {
		var err error

		rect, err := ReadRectangle(reader)
		if err != nil {
			return nil, err
		}

		switch rect.EncodingType {
		case RawEncoding:
			err = DecodeRawEncoding(reader, &rect)
		case TightEncoding:
			err = tight.Decode(reader, &rect)
		case DesktopSize:
			err = DecodeDesktopSizeEncoding(reader, &rect)
		default:
			err = fmt.Errorf("unaccepted encoding: %d", rect.EncodingType)
		}

		if err != nil {
			return nil, err
		}

		update.Rectangles = append(update.Rectangles, rect)
	}

	return update, nil
}

// serveTranscode serves the recordings in dir on ln and then calls run with
// the URL of the recording name and out. The listener is ready before run
// starts. serveTranscode closes ln when run returns.
func serveTranscode(ln net.Listener, dir, name, out string, run func(src, out string) error) error {
	srv := &http.Server{Handler: &playbackServer{http.Dir(dir)}}
	defer srv.Close()

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Error("serve recordings: %v", err)
		}
	}()

	host := ln.Addr().String()
	if addr, ok := ln.Addr().(*net.TCPAddr); ok {
		host = net.JoinHostPort("localhost", strconv.Itoa(addr.Port))
	}
	src := &url.URL{Scheme: "http", Host: host, Path: "/" + name}

	return run(src.String(), out)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 && flag.NArg() != 2 {
		usage()
		os.Exit(1)
	}

	log.Init()

	addr := ":" + strconv.Itoa(*f_port)
	log.Info("serving recordings from %s on %s", flag.Arg(0), addr)

	switch flag.NArg() {
	case 1: // just serve a directory and mjpeg streams
		// Ensure that the first arg is an existent directory
		if fi, err := os.Stat(flag.Arg(0)); err != nil || !fi.IsDir() {
			fmt.Print("Invalid argument: must be an existent directory\n\n")
			usage()
			os.Exit(1)
		}
		http.Handle("/", &playbackServer{http.Dir(flag.Arg(0))})
		log.Fatalln(http.ListenAndServe(addr, nil))
	case 2: // transcode with ffmpeg
		in := flag.Arg(0)
		out := flag.Arg(1)
		log.Debug("transcoding %v to %v", in, out)

		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalln(err)
		}

		err = serveTranscode(ln, filepath.Dir(in), filepath.Base(in), out, transcode)
		if err != nil {
			log.Fatalln(err)
		}
	default:
		usage()
		os.Exit(1)
	}
}
