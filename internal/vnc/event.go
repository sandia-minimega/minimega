// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"image"

	// supported image formats
	_ "image/jpeg"
	_ "image/png"
)

// Events can be written to a vnc connect
type Event interface {
	Write(w io.Writer) error
}

// WaitForItEvent is a pseudo event indicating that we should wait for an image
// to appear on the screen. If click is true, we should click on the center of
// the image if it appears.
type WaitForItEvent struct {
	Source   string
	Template image.Image
	Timeout  time.Duration
	Click    bool
}

// LoadFileEvent is a pseudo event indicating that we should start reading
// events from a different file.
type LoadFileEvent struct {
	File string
}

const (
	keyEventFmt     = "KeyEvent,%t,%s"
	pointerEventFmt = "PointerEvent,%d,%d,%d"
)

// playbackMask is the part of a recorded button mask that playback can send.
// The playback connection does not negotiate ExtendedMouseButtons (-316).
const playbackMask = 0x7f

// kbPointerEvent is a PointerEvent line of a kb file. Mask holds the 9-bit
// button mask, with Back and Forward in bits 7 and 8.
type kbPointerEvent struct {
	Mask      uint16
	XPosition uint16
	YPosition uint16
}

func (e *kbPointerEvent) String() string {
	return fmt.Sprintf(pointerEventFmt, e.Mask, e.XPosition, e.YPosition)
}

// Write sends a 6-byte PointerEvent with Mask & playbackMask.
func (e *kbPointerEvent) Write(w io.Writer) error {
	return (&PointerEvent{
		ButtonMask: uint8(e.Mask & playbackMask),
		XPosition:  e.XPosition,
		YPosition:  e.YPosition,
	}).Write(w)
}

// dropped returns the mask bits that Write does not send.
func (e *kbPointerEvent) dropped() uint16 {
	return e.Mask &^ playbackMask
}

func parseEvent(cmd string) (interface{}, error) {
	fields := strings.Split(cmd, ",")

	switch fields[0] {
	case "KeyEvent":
		if len(fields) != 3 {
			return nil, fmt.Errorf("expected 2 values for KeyEvent, got %v", len(fields)-1)
		}

		down, err := strconv.ParseBool(fields[1])
		if err != nil {
			return nil, fmt.Errorf("invalid KeyEvent: %v", err)
		}

		e := &KeyEvent{}
		if down {
			e.DownFlag = 1
		}

		e.Key, err = xStringToKeysym(fields[2])
		if err != nil {
			_, err = fmt.Sscanf(fields[2], "%U", &e.Key)
			if err != nil {
				return nil, fmt.Errorf("unknown key: `%s`", fields[2])
			}
		}

		return e, nil
	case "PointerEvent":
		if len(fields) != 4 {
			return nil, fmt.Errorf("expected 3 values for PointerEvent, got %v", len(fields)-1)
		}

		mask, err := strconv.ParseUint(fields[1], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid PointerEvent: %v", err)
		}

		x, err := strconv.ParseUint(fields[2], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid PointerEvent: %v", err)
		}

		y, err := strconv.ParseUint(fields[3], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid PointerEvent: %v", err)
		}

		e := &kbPointerEvent{
			Mask:      uint16(mask),
			XPosition: uint16(x),
			YPosition: uint16(y),
		}

		return e, nil
	case "LoadFile":
		if len(fields) != 2 {
			return nil, fmt.Errorf("expected 1 values for LoadFile, got %v", len(fields)-1)
		}

		e := &LoadFileEvent{
			File: fields[1],
		}

		return e, nil
	case "WaitForIt", "ClickIt", "ClickItEvent":
		if len(fields) != 3 {
			return nil, fmt.Errorf("expected 2 values for %v, got %v", fields[0], len(fields)-1)
		}

		timeout, err := parseDuration(fields[1])
		if err != nil {
			return nil, fmt.Errorf("invalid timeout for %v: %v", fields[0], err)
		}

		e := &WaitForItEvent{
			Timeout: timeout,
			Click:   fields[0] != "WaitForIt",
			Source:  fields[2],
		}

		// TODO: cache image?
		template, isBase64, err := loadTemplate(fields[2])
		if err != nil {
			return nil, err
		}

		// truncate for sanity
		if isBase64 && len(e.Source) > 32 {
			e.Source = e.Source[:29] + "..."
		}

		e.Template = template

		return e, nil
	}

	return nil, errors.New("invalid event specified")
}

// openTemplate opens a template file. The fuzz test replaces it.
var openTemplate = os.Open

// maxTemplatePixels is the largest template image, in pixels.
const maxTemplatePixels = 4096 * 4096

// loadTemplate reads the template image in the file arg. If arg is not a
// file, it decodes arg as a base64 image and returns true.
func loadTemplate(arg string) (image.Image, bool, error) {
	f, err := openTemplate(arg)
	if err == nil {
		defer f.Close()

		b, err := io.ReadAll(io.LimitReader(f, maxKBLine))
		if err != nil {
			return nil, false, err
		}

		img, err := decodeTemplate(b)
		if err != nil {
			return nil, false, fmt.Errorf("template %v: %w", arg, err)
		}
		return img, false, nil
	}

	b, err2 := base64.StdEncoding.DecodeString(arg)
	if err2 != nil {
		// arg is not base64, so it must be a path to a file
		return nil, false, err
	}

	img, err := decodeTemplate(b)
	if err != nil {
		return nil, true, fmt.Errorf("base64 template: %w", err)
	}
	return img, true, nil
}

// decodeTemplate decodes a PNG or JPEG image of at most maxTemplatePixels.
func decodeTemplate(b []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 65535 || cfg.Height > 65535 || cfg.Width*cfg.Height > maxTemplatePixels {
		return nil, fmt.Errorf("image size %vx%v is not supported", cfg.Width, cfg.Height)
	}

	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

func parseDuration(s string) (time.Duration, error) {
	// unitless integer is assumed to be in nanoseconds
	if v, err := strconv.Atoi(s); err == nil {
		return time.Duration(v) * time.Nanosecond, nil
	}

	return time.ParseDuration(s)
}

// getDuration returns the duration of a given playback file. It skips the
// lines that playFile skips, and playFile logs them.
func getDuration(f *os.File) time.Duration {
	// go back to the beginning of the file
	defer f.Seek(0, io.SeekStart)

	var d time.Duration

	scanner := newKBScanner(f)
	for scanner.Scan() {
		s := strings.SplitN(scanner.Text(), ":", 2)
		// Ignore blank and malformed lines, and comments
		if len(s) != 2 || strings.HasPrefix(s[0], "#") {
			continue
		}

		if v, err := parseDuration(s[0]); err == nil {
			d += v
		}
	}

	return d
}
