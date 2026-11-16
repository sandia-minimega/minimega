// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func ReadPixel(reader io.Reader) (pixel color.RGBA, err error) {
	var n int

	bytesPerPixel := pixelFormat.BitsPerPixel / uint8(8)
	buf := make([]byte, bytesPerPixel)

	n, err = io.ReadFull(reader, buf)
	if err != nil {
		return
	} else if uint8(n) != bytesPerPixel {
		return pixel, fmt.Errorf("unable to read full pixel from reader (%d/%d)", n, bytesPerPixel)
	}

	var raw uint32
	if pixelFormat.BigEndianFlag != 0 {
		raw = binary.BigEndian.Uint32(buf)
	} else {
		raw = binary.LittleEndian.Uint32(buf)
	}

	if pixelFormat.TrueColorFlag != 0 {
		pixel.R = uint8((raw >> uint32(pixelFormat.RedShift)) & uint32(pixelFormat.RedMax))
		pixel.G = uint8((raw >> uint32(pixelFormat.GreenShift)) & uint32(pixelFormat.GreenMax))
		pixel.B = uint8((raw >> uint32(pixelFormat.BlueShift)) & uint32(pixelFormat.BlueMax))
		pixel.A = 255
	} else {
		return pixel, errors.New("mama never taught me how to decode untrue colors")
	}

	return
}

// maxRectDim is the largest X+Width and Y+Height of a rectangle. The canvas
// grows only to hold rectangles, so it has the same limit.
// The QEMU VNC limit is not checked here; 8192 is larger than usual displays.
const maxRectDim = 8192

// maxUpdatePixels is the largest total area of the rectangles with pixels in
// one update: one full desktop. It is a variable for the tests.
var maxUpdatePixels = maxRectDim * maxRectDim

// ReadRectangle reads a rectangle header. It refuses a rectangle with pixels
// that has an area larger than maxPixels, before it allocates the image.
func ReadRectangle(buf io.Reader, maxPixels int) (rect Rectangle, err error) {
	// Try to read all the fields
	var X, Y, Width, Height uint16

	if err2 := binary.Read(buf, binary.BigEndian, &X); err2 != nil {
		err = errors.New("unable to decode rect X pos")
	} else if err2 := binary.Read(buf, binary.BigEndian, &Y); err2 != nil {
		err = errors.New("unable to decode rect Y pos")
	} else if err2 := binary.Read(buf, binary.BigEndian, &Width); err2 != nil {
		err = errors.New("unable to decode rect width")
	} else if err2 := binary.Read(buf, binary.BigEndian, &Height); err2 != nil {
		err = errors.New("unable to decode rect height")
	} else if err2 := binary.Read(buf, binary.BigEndian, &rect.EncodingType); err2 != nil {
		err = errors.New("unable to decode rect encoding type")
	}

	if err != nil {
		return
	}

	log.Debug("rectangle: %d x %d at (%d, %d)", Width, Height, X, Y)

	if int(X)+int(Width) > maxRectDim || int(Y)+int(Height) > maxRectDim {
		return rect, fmt.Errorf("rectangle %d x %d at (%d, %d) is outside the %d x %d limit", Width, Height, X, Y, maxRectDim, maxRectDim)
	}

	r := image.Rect(int(X), int(Y), int(X)+int(Width), int(Y)+int(Height))

	// A DesktopSize rectangle has no pixel data.
	if rect.EncodingType == DesktopSize {
		rect.RGBA = &image.RGBA{Rect: r}
		return
	}

	if r.Dx()*r.Dy() > maxPixels {
		return rect, fmt.Errorf("rectangle %d x %d at (%d, %d) is larger than the %d pixels left for the update", Width, Height, X, Y, maxPixels)
	}

	rect.RGBA = image.NewRGBA(r)

	return
}
