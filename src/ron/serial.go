// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"encoding/gob"
	"fmt"
	"goserial"
	"io"
	"io/ioutil"
	log "minilog"
	"net"
	"os"
	"path/filepath"
)

const (
	BAUDRATE = 115200
)

type serialFile struct {
	Error string
	File  []byte
}

func init() {
	gob.Register(serialFile{})
}

func (r *Ron) serialDial() error {
	c := &serial.Config{
		Name: r.serialPath,
		Baud: BAUDRATE,
	}

	s, err := serial.OpenPort(c)
	if err != nil {
		return err
	}

	r.serialClientHandle = s

	return nil
}

func (r *Ron) serialHeartbeat(h *hb) (map[int]*Command, error, bool) {
	enc := gob.NewEncoder(r.serialClientHandle)
	dec := gob.NewDecoder(r.serialClientHandle)

	err := enc.Encode(h)
	if err != nil {
		return nil, err, false
	}

	newCommands := make(map[int]*Command)

	err = dec.Decode(&newCommands)
	if err != nil {
		log.Errorln("error decoding response over serial, reconnecting")

		closeErr := r.serialClientHandle.Close()
		if closeErr != nil {
			log.Fatalln(closeErr)
		}

		r.serialClientHandle = nil
		redialErr := r.serialDial()
		if redialErr != nil {
			log.Fatalln(redialErr)
		}

		return nil, err, true
	}

	return newCommands, nil, true
}

func (r *Ron) GetActiveSerialPorts() []string {
	r.serialLock.Lock()
	defer r.serialLock.Unlock()

	var ret []string
	for k, _ := range r.masterSerialConns {
		ret = append(ret, k)
	}

	return ret
}

func (r *Ron) SerialGetFile(filename string) ([]byte, error) {
	log.Debug("SerialGetFile: %v", filename)

	enc := gob.NewEncoder(r.serialClientHandle)
	dec := gob.NewDecoder(r.serialClientHandle)

	h := hb{File: filename}

	err := enc.Encode(&h)
	if err != nil {
		return nil, err
	}

	var sf serialFile
	err = dec.Decode(&sf)
	if err != nil {
		return nil, err
	}

	var errRet error
	if sf.Error != "" {
		errRet = fmt.Errorf(sf.Error)
	}

	return sf.File, errRet
}

// Dial a client serial port. Used by a master ron node only.
func (r *Ron) SerialDialClient(path string) error {
	log.Debug("SerialDialClient: %v", path)

	if r.mode != MODE_MASTER {
		log.Fatalln("SerialDialClient must be in master mode")
	}

	r.serialLock.Lock()
	defer r.serialLock.Unlock()

	// are we already connected to this client?
	if _, ok := r.masterSerialConns[path]; ok {
		return fmt.Errorf("already connected to serial client %v", path)
	}

	// connect!
	s, err := net.Dial("unix", path)
	if err != nil {
		return err
	}

	r.masterSerialConns[path] = s

	go r.serialClientHandler(path)

	return nil
}

func (r *Ron) serialClientHandler(path string) {
	log.Debug("serialClientHandler: %v", path)

	r.serialLock.Lock()
	c, ok := r.masterSerialConns[path]
	r.serialLock.Unlock()

	if !ok {
		log.Fatal("could not access client: %v", path)
	}

	for {
		enc := gob.NewEncoder(c)
		dec := gob.NewDecoder(c)
		var h hb
		err := dec.Decode(&h)
		if err != nil {
			if err != io.EOF {
				log.Errorln(err)
			}
			break
		}

		if h.File != "" {
			log.Debug("file get heartbeat: %v", h.File)

			var sf serialFile

			// simply encode the file back if it exists
			filename := filepath.Join(r.path, h.File)
			info, err := os.Stat(filename)
			if err != nil {
				e := fmt.Errorf("file %v does not exist: %v", filename, err)
				sf.Error = e.Error()
				log.Errorln(e)
			} else if info.IsDir() {
				e := fmt.Errorf("file %v is a directory", filename)
				sf.Error = e.Error()
				log.Errorln(e)
			} else {
				// read the file
				sf.File, err = ioutil.ReadFile(filename)
				if err != nil {
					e := fmt.Errorf("file %v: %v", filename, err)
					sf.Error = e.Error()
					log.Errorln(e)
				}
			}

			err = enc.Encode(&sf)
			if err != nil {
				if err != io.EOF {
					log.Errorln(err)
				}
				break
			}
			continue
		}

		log.Debug("heartbeat from %v", h.UUID)

		// process the heartbeat in a goroutine so we can send the command list back faster
		go r.masterHeartbeat(&h)

		// send the command list back
		err = enc.Encode(r.commands)
		if err != nil {
			if err != io.EOF {
				log.Errorln(err)
			}
			break
		}
	}

	// remove this path from the list of connected serial ports
	log.Debug("disconnecting serial client: %v", path)

	r.serialLock.Lock()
	delete(r.masterSerialConns, path)
	r.serialLock.Unlock()
}
