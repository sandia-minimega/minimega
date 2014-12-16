package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"time"
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

func readFile(fname string) (chan *FramebufferUpdate, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}

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
			log.Println("error decoding recording:", err)
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

	//log.Println("message type:", mType)

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

	//log.Println("number of rectangles:", numRects)

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

func usage() {
	fmt.Printf("USAGE: %s [OPTION]... DIR\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	http.HandleFunc("/", handler)
	http.ListenAndServe(":7777", nil)
}

func handler(w http.ResponseWriter, r *http.Request) {
	updateChan, _ := readFile(flag.Arg(0))
	imageChan := make(chan image.Image, 10)

	// gorountine to rebuild the images
	go func() {
		var X, Y int

		prev := time.Now()
		img := image.NewRGBA(image.Rect(0, 0, X, Y))

		// for each jpeg image
		for update := range updateChan {
			// Check if the resolution has changed
			last := update.Rectangles[len(update.Rectangles)-1]
			if last.EncodingType == DesktopSize {
				X = last.Rect.Max.X
				Y = last.Rect.Max.Y
			}

			nimg := image.NewRGBA(image.Rect(0, 0, X, Y))

			// Copy in the previous image
			dr := image.Rectangle{img.Rect.Min, img.Rect.Max}
			draw.Draw(nimg, dr, img, img.Rect.Min, draw.Src)

			for _, r := range update.Rectangles {
				dr := image.Rectangle{r.Rect.Min, r.Rect.Max}
				//log.Printf("drawing in rectangle at %#v\n", dr)
				draw.Draw(nimg, dr, r, r.Rect.Min, draw.Src)
			}

			offset := time.Now().Sub(prev).Nanoseconds()
			prev = time.Now()

			if offset < update.Offset {
				// Sleep until the next image should be served
				time.Sleep(time.Duration(update.Offset - offset))
			} else {
				//log.Println("warning: longer to replay images than record them")
			}

			imageChan <- nimg
			img = nimg
		}

		close(imageChan)
	}()

	mh := make(textproto.MIMEHeader)
	mh.Set("Content-Type", "image/jpeg")

	m := multipart.NewWriter(w)

	h := w.Header()
	boundary := m.Boundary()
	h.Set("Content-type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", boundary))

	// encode and send the image
	var buf bytes.Buffer
	for image := range imageChan {
		buf.Reset()

		//log.Printf("writing image: %v", image.Bounds())
		err := jpeg.Encode(&buf, image, nil)
		if err != nil {
			log.Printf("unable to encode jpeg: %v", err)
			break
		}

		mh.Set("Content-length", fmt.Sprintf("%d", buf.Len()))
		fm, err := m.CreatePart(mh)
		if err != nil {
			log.Printf("unable to create multipart: %v", err)
			return
		}
		_, err = io.Copy(fm, &buf)
		if err != nil {
			log.Printf("unable to write multipart: %v", err)
			break
		}
	}
}
