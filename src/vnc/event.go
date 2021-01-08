// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	log "minilog"
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

		mask, err := strconv.ParseUint(fields[1], 10, 8)
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

		e := &PointerEvent{
			ButtonMask: uint8(mask),
			XPosition:  uint16(x),
			YPosition:  uint16(y),
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
	case "WaitForIt", "ClickItEvent":
		if len(fields) != 3 {
			return nil, fmt.Errorf("expected 2 values for %v, got %v", fields[0], len(fields)-1)
		}

		timeout, err := parseDuration(fields[1])
		if err != nil {
			return nil, fmt.Errorf("invalid timeout for %v: %v", fields[0], err)
		}

		e := &WaitForItEvent{
			Timeout: timeout,
			Click:   fields[0] == "ClickItEvent",
			Source:  fields[2],
		}

		// reader for image
		var r io.Reader

		b, err := base64.StdEncoding.DecodeString(fields[2])
		if err == nil {
			// successfully decoded as base64-encoded image
			r = bytes.NewReader(b)

			// truncate for sanity
			if len(e.Source) > 32 {
				e.Source = e.Source[:29] + "..."
			}
		} else {
			// argument must be a path to a file
			f, err := os.Open(fields[2])
			if err != nil {
				return nil, err
			}
			defer f.Close()

			r = f
		}

		// TODO: cache image?
		template, _, err := image.Decode(r)
		if err != nil {
			return nil, err
		}

		e.Template = template

		return e, nil
	}

	return nil, errors.New("invalid event specified")
}

func parseDuration(s string) (time.Duration, error) {
	// unitless integer is assumed to be in nanoseconds
	if v, err := strconv.Atoi(s); err == nil {
		return time.Duration(v) * time.Nanosecond, nil
	}

	return time.ParseDuration(s)
}

// getDuration returns the duration of a given playback file
func getDuration(f *os.File) time.Duration {
	// go back to the beginning of the file
	defer f.Seek(0, 0)

	d := 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := strings.SplitN(scanner.Text(), ":", 2)
		// Ignore blank and malformed lines
		if len(s) != 2 {
			log.Debug("malformed vnc statement: %s", scanner.Text())
			continue
		}

		// Ignore comments in the vnc file
		if s[0] == "#" {
			continue
		}

		i, err := strconv.Atoi(s[0])
		if err != nil {
			log.Errorln(err)
			return 0
		}
		d += i
	}

	return time.Duration(d) * time.Nanosecond
}
