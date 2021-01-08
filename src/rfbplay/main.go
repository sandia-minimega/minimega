// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	log "minilog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type Rectangle struct {
	*image.RGBA
	EncodingType int32
}

type FramebufferUpdate struct {
	// How long to wait before this update should be applied
	Offset int64
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

func readFile(f http.File) (chan *FramebufferUpdate, error) {
	gzipReader, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	bufioReader := bufio.NewReader(gzipReader)
	reader := NewRecordingReader(bufioReader)
	output := make(chan *FramebufferUpdate)

	go func() {
		defer f.Close()
		defer close(output)

		var err error
		for err == nil {
			err = readUpdate(reader, output)
		}

		if err != nil && err != io.EOF {
			log.Errorln("error decoding recording:", err)
		}
	}()

	return output, nil
}

func readUpdate(reader *RecordingReader, output chan *FramebufferUpdate) error {
	// Decode the message type
	var mType uint8
	if err := binary.Read(reader, binary.BigEndian, &mType); err != nil {
		if err == io.EOF {
			return err // No more updates
		}
		return errors.New("unable to decode message type")
	}

	log.Debugln("message type:", mType)

	// Skip message that aren't framebuffer updates
	if mType != 0 {
		return errors.New("unable to decode, found non-framebuffer update")
	}

	update := FramebufferUpdate{}
	update.Offset = reader.Offset()

	// Skip the one byte of padding
	if _, err := reader.Read(make([]byte, 1)); err != nil {
		return errors.New("unable to skip padding")
	}

	// Decode the number of rectangles
	var numRects uint16
	if err := binary.Read(reader, binary.BigEndian, &numRects); err != nil {
		return errors.New("unable to decode number of rectangles")
	}

	log.Debugln("number of rectangles:", numRects)

	// Read all the rectangles
	for len(update.Rectangles) < int(numRects) {
		var err error

		rect, err := ReadRectangle(reader)
		if err != nil {
			return err
		}

		switch rect.EncodingType {
		case RawEncoding:
			err = DecodeRawEncoding(reader, &rect)
		case TightEncoding:
			err = DecodeTightEncoding(reader, &rect)
		case DesktopSize:
			err = DecodeDesktopSizeEncoding(reader, &rect)
		default:
			err = fmt.Errorf("unaccepted encoding: %d", rect.EncodingType)
		}

		if err != nil {
			return err
		}

		update.Rectangles = append(update.Rectangles, rect)
	}

	output <- &update
	return nil
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
		http.ListenAndServe(addr, nil)
	case 2: // transcode with ffmpeg
		in := flag.Arg(0)
		out := flag.Arg(1)
		log.Debug("transcoding %v to %v", in, out)

		path := filepath.Dir(in)
		fname := filepath.Base(in)
		http.Handle("/", &playbackServer{http.Dir(path)})
		go http.ListenAndServe(addr, nil)

		err := transcode(fname, out)
		if err != nil {
			log.Fatalln(err)
		}
	default:
		usage()
		os.Exit(1)
	}
}
