// Copyright (2019) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package vnc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	log "minilog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Event
type Event interface {
	Write(w io.Writer) error
}

const (
	keyEventFmt     = "KeyEvent,%t,%s"
	pointerEventFmt = "PointerEvent,%d,%d,%d"
	loadEventFmt    = "LoadFile,%s"
)

func parseEvent(cmd string) (interface{}, error) {
	if e, err := parseKeyEvent(cmd); err == nil {
		return e, err
	} else if e, err := parsePointerEvent(cmd); err == nil {
		return e, err
	} else if e, err := parseLoadFileEvent(cmd); err == nil {
		return e, err
	}

	return nil, errors.New("invalid event specified")
}

func parseLoadFileEvent(arg string) (string, error) {
	var filename string

	_, err := fmt.Sscanf(arg, loadEventFmt, &filename)
	if err != nil {
		return "", err
	}

	return filename, nil
}

func parseKeyEvent(arg string) (*KeyEvent, error) {
	var key string
	var down bool

	_, err := fmt.Sscanf(arg, keyEventFmt, &down, &key)
	if err != nil {
		return nil, err
	}

	m := &KeyEvent{}

	m.Key, err = xStringToKeysym(key)
	if err != nil {
		fmt.Println(err.Error())
		_, err = fmt.Sscanf(key, "%U", &m.Key)
		if err != nil {
			fmt.Println(err.Error())
			return nil, fmt.Errorf("unknown key: `%s`", key)
		}
	}

	if down {
		m.DownFlag = 1
	}

	return m, nil
}

func parsePointerEvent(arg string) (*PointerEvent, error) {
	m := &PointerEvent{}

	_, err := fmt.Sscanf(arg, pointerEventFmt, &m.ButtonMask, &m.XPosition, &m.YPosition)
	if err != nil {
		return nil, err
	}

	return m, nil
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
