// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// iomeshage is a file transfer layer for meshage
//
// Files are stored in a predetermined directory structure. When a particular
// meshage node needs a file, it polls nodes looking for that file, looking at
// shortest path nodes first. The node with the file and the fewest hops will
// transfer the file to the requesting node.
package iomeshage

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"meshage"
	log "minilog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	MAX_ATTEMPTS = 3
	QUEUE_LEN    = 3
)

// IOMeshage object, which must have a base path to serve files on and a
// meshage node.
type IOMeshage struct {
	base         string                // base path for serving files
	node         *meshage.Node         // meshage node to use
	Messages     chan *meshage.Message // Incoming messages from meshage
	TIDs         map[int64]chan *IOMMessage
	tidLock      sync.Mutex
	transfers    map[string]*Transfer // current transfers
	drainLock    sync.RWMutex
	transferLock sync.RWMutex
	queue        chan bool
}

// FileInfo object. Used by the calling API to describe existing files.
type FileInfo struct {
	Name string
	Dir  bool
	Size int64
}

// Transfer describes an in-flight transfer.
type Transfer struct {
	Dir      string         // temporary directory hold the file parts
	Filename string         // file name
	Parts    map[int64]bool // completed parts
	NumParts int            // total number of parts for this file
	Inflight int64          // currently in-flight part, -1 if none
	Queued   bool
}

var (
	timeout = time.Duration(30 * time.Second)
)

// New returns a new iomeshage object service base directory b on meshage node
// n
func New(base string, node *meshage.Node) (*IOMeshage, error) {
	base = filepath.Clean(base)
	log.Debug("new iomeshage node on base %v", base)
	err := os.MkdirAll(base, 0755)

	r := &IOMeshage{
		base:      base,
		node:      node,
		Messages:  make(chan *meshage.Message, 1024),
		TIDs:      make(map[int64]chan *IOMMessage),
		transfers: make(map[string]*Transfer),
		queue:     make(chan bool, QUEUE_LEN),
	}

	go r.handleMessages()

	return r, err
}

// List files and directories starting at iom.base+dir
func (iom *IOMeshage) List(dir string) ([]FileInfo, error) {
	dir = iom.dirPrep(dir)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var ret []FileInfo
	for _, f := range files {
		i := FileInfo{
			Name: f.Name(),
			Dir:  f.IsDir(),
			Size: f.Size(),
		}
		ret = append(ret, i)
	}
	return ret, nil
}

// search the mesh for the file/glob/directory, returning a slice of string
// matches. The search includes local matches.
func (iom *IOMeshage) Info(file string) []string {
	var ret []string

	// search locally
	files, _, _ := iom.fileInfo(filepath.Join(iom.base, file))
	ret = append(ret, files...)

	// search the mesh
	TID := genTID()
	c := make(chan *IOMMessage)
	err := iom.registerTID(TID, c)
	defer iom.unregisterTID(TID)

	if err != nil {
		// a collision in int64, we should tell someone about this
		log.Fatalln(err)
	}

	m := &IOMMessage{
		From:     iom.node.Name(),
		Type:     TYPE_INFO,
		Filename: file,
		TID:      TID,
	}
	recipients, err := iom.node.Broadcast(m)
	if err != nil {
		log.Errorln(err)
		return nil
	}
	if log.WillLog(log.DEBUG) {
		log.Debug("sent info request to %v nodes", len(recipients))
	}

	// wait for n responses, or a timeout
	for i := 0; i < len(recipients); i++ {
		select {
		case resp := <-c:
			if log.WillLog(log.DEBUG) {
				log.Debugln("got response: ", resp)
			}
			if resp.ACK {
				if log.WillLog(log.DEBUG) {
					log.Debugln("got info from: ", resp.From)
				}
				if len(resp.Glob) == 0 {
					// exact match unless the exact match is the original glob
					if !strings.Contains(resp.Filename, "*") {
						ret = append(ret, resp.Filename)
					}
				} else {
					ret = append(ret, resp.Glob...)
				}
			}
		case <-time.After(timeout):
			log.Errorln(fmt.Errorf("timeout"))
			return nil
		}
	}

	return ret
}

// Retrieve a file from the shortest path node that has it. Get blocks until
// the file transfer is begins or errors out. If the file specified is a
// directory, the entire directory will be recursively transferred.
// If the file already exists on this node, Get will return immediately with no
// error.
func (iom *IOMeshage) Get(file string) error {
	// is this a directory or a glob
	fi, err := os.Stat(filepath.Join(iom.base, file))
	if err == nil && !fi.IsDir() {
		return nil
	}

	// is this file already in flight?
	iom.transferLock.RLock()
	if _, ok := iom.transfers[file]; ok {
		iom.transferLock.RUnlock()
		return fmt.Errorf("file already in flight")
	}
	iom.transferLock.RUnlock()

	// find the file somewhere in the mesh
	TID := genTID()
	c := make(chan *IOMMessage)
	err = iom.registerTID(TID, c)
	defer iom.unregisterTID(TID)

	if err != nil {
		// a collision in int64, we should tell someone about this
		log.Fatalln(err)
	}

	m := &IOMMessage{
		From:     iom.node.Name(),
		Type:     TYPE_INFO,
		Filename: file,
		TID:      TID,
	}
	recipients, err := iom.node.Broadcast(m)
	if err != nil {
		return err
	}
	if log.WillLog(log.DEBUG) {
		log.Debug("sent info request to %v nodes", len(recipients))
	}

	var info []*IOMMessage
	var gotInfo bool
	// wait for n responses, or a timeout
	for i := 0; i < len(recipients); i++ {
		select {
		case resp := <-c:
			if log.WillLog(log.DEBUG) {
				log.Debugln("got response: ", resp)
			}
			if resp.ACK {
				if log.WillLog(log.DEBUG) {
					log.Debugln("got info from: ", resp.From)
				}
				info = append(info, resp)
				gotInfo = true
			}
		case <-time.After(timeout):
			return fmt.Errorf("timeout")
		}
	}
	if !gotInfo {
		return fmt.Errorf("file not found")
	}

	inflight := make(map[string]bool)

	for _, v := range info {
		// is this a single file or a directory/blob?
		if len(v.Glob) == 0 {
			if _, ok := inflight[v.Filename]; ok {
				continue
			}

			if log.WillLog(log.DEBUG) {
				log.Debug("found file on node %v with %v parts", v.From, v.Part)
			}

			// create a transfer object
			tdir, err := ioutil.TempDir(iom.base, "transfer_")
			if err != nil {
				log.Errorln(err)
				return err
			}
			iom.transferLock.Lock()
			iom.transfers[v.Filename] = &Transfer{
				Dir:      tdir,
				Filename: v.Filename,
				Parts:    make(map[int64]bool),
				NumParts: int(v.Part),
				Inflight: -1,
				Queued:   true,
			}
			iom.transferLock.Unlock()

			go iom.getParts(v.Filename, v.Part, v.Perm)

			inflight[v.Filename] = true
		} else {
			// call Get on each of the constituent files, queued in a random order

			// fisher-yates shuffle
			s := rand.NewSource(time.Now().UnixNano())
			r := rand.New(s)
			for i := int64(len(v.Glob)) - 1; i > 0; i-- {
				j := r.Int63n(i + 1)
				t := v.Glob[j]
				v.Glob[j] = v.Glob[i]
				v.Glob[i] = t
			}

			for _, x := range v.Glob {
				if _, ok := inflight[x]; ok {
					continue
				}
				err := iom.Get(x)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Get a file with numParts parts. getParts will randomize the order of the
// parts to maximize the distributed transfer behavior of iomeshage when used
// at scale.
func (iom *IOMeshage) getParts(filename string, numParts int64, perm os.FileMode) {
	defer iom.destroyTempTransfer(filename)

	// corner case - empty file
	if numParts == 0 {
		log.Debug("file %v has 0 parts, creating empty file", filename)

		// create subdirectories
		fullPath := filepath.Join(iom.base, filename)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			log.Errorln(err)
			return
		}
		f, err := os.Create(fullPath)
		if err != nil {
			log.Errorln(err)
			return
		}
		f.Close()
		log.Debug("changing permissions: %v %v", fullPath, perm)
		err = os.Chmod(fullPath, perm)
		if err != nil {
			log.Errorln(err)
		}
		return
	}

	// create a random list of parts to grab
	var parts []int64
	var i int64
	for i = 0; i < numParts; i++ {
		parts = append(parts, i)
	}

	// fisher-yates shuffle
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	for i = numParts - 1; i > 0; i-- {
		j := r.Int63n(i + 1)
		t := parts[j]
		parts[j] = parts[i]
		parts[i] = t
	}

	// get in line
	iom.queue <- true
	defer func() {
		<-iom.queue
	}()

	iom.transferLock.Lock()
	iom.transfers[filename].Queued = false
	iom.transferLock.Unlock()

	for _, p := range parts {
		// did I already get this part via another node's request?
		iom.transferLock.Lock()
		if iom.transfers[filename].Parts[p] {
			iom.transferLock.Unlock()
			continue
		}
		iom.transfers[filename].Inflight = p
		iom.transferLock.Unlock()

		// attempt to get this part up to MAX_ATTEMPTS attempts
		for attempt := 0; attempt < MAX_ATTEMPTS; attempt++ {
			TID := genTID()
			c := make(chan *IOMMessage)
			err := iom.registerTID(TID, c)

			if err != nil {
				// a collision in int64, we should tell someone about this
				log.Fatalln(err)
			}

			if log.WillLog(log.DEBUG) {
				log.Debug("transferring filepart %v:%v, attempt %v", filename, p, attempt)
			}
			if attempt > 0 {
				// we're most likely issuing multiple attempts because of heavy traffic, wait a bit for things to calm down
				time.Sleep(timeout)
			}
			m := &IOMMessage{
				From:     iom.node.Name(),
				Type:     TYPE_WHOHAS,
				Filename: filename,
				TID:      TID,
				Part:     p,
			}

			recipients, err := iom.node.Broadcast(m)
			if err != nil {
				log.Errorln(err)
				iom.unregisterTID(TID)
				continue
			}
			if log.WillLog(log.DEBUG) {
				log.Debug("sent info request to %v nodes", len(recipients))
			}

			var info *IOMMessage
			var gotPart bool
			var timeoutCount int
			// wait for n responses, or a timeout
		IOMESHAGE_WHOHAS_LOOP:
			for i := 0; i < len(recipients); i++ {
				select {
				case resp := <-c:
					if log.WillLog(log.DEBUG) {
						log.Debugln("got response: ", resp)
					}
					if resp.ACK {
						if log.WillLog(log.DEBUG) {
							log.Debugln("got partInfo from: ", resp.From)
						}
						info = resp
						gotPart = true
						break IOMESHAGE_WHOHAS_LOOP
					}
				case <-time.After(timeout):
					log.Errorln("timeout")
					timeoutCount++

					if timeoutCount == MAX_ATTEMPTS {
						log.Debugln("too many timeouts")
						break IOMESHAGE_WHOHAS_LOOP
					}
					continue
				}
			}
			if !gotPart {
				log.Errorln("part not found")
				iom.unregisterTID(TID)
				continue
			}

			if log.WillLog(log.DEBUG) {
				log.Debug("found part %v on node %v", info.Part, info.From)
			}

			// transfer this part
			err = iom.Xfer(m.Filename, info.Part, info.From)
			if err != nil {
				log.Errorln(err)
				iom.unregisterTID(TID)
				continue
			}
			iom.transferLock.Lock()
			iom.transfers[filename].Parts[p] = true
			iom.transferLock.Unlock()
			iom.unregisterTID(TID)
			break
		}
		iom.transferLock.RLock()
		if !iom.transfers[filename].Parts[p] {
			log.Error("could not transfer filepart %v:%v after %v attempts", filename, p, MAX_ATTEMPTS)
			iom.transferLock.RUnlock()
			return
		}
		iom.transferLock.RUnlock()
	}

	// copy the parts into the whole file
	iom.transferLock.RLock()
	t := iom.transfers[filename]
	iom.transferLock.RUnlock()
	tfile, err := ioutil.TempFile(t.Dir, "cat_")
	if err != nil {
		log.Errorln(err)
	}

	for i = 0; i < numParts; i++ {
		fname := fmt.Sprintf("%v/%v.part_%v", t.Dir, filepath.Base(filename), i)
		fpart, err := os.Open(fname)
		if err != nil {
			log.Errorln(err)
			tfile.Close()
			return
		}
		io.Copy(tfile, fpart)
		fpart.Close()
	}
	name := tfile.Name()
	tfile.Close()

	// create subdirectories
	fullPath := filepath.Join(iom.base, filename)
	err = os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err != nil {
		log.Errorln(err)
		return
	}
	os.Rename(name, fullPath)

	log.Debug("changing permissions: %v %v", fullPath, perm)
	err = os.Chmod(fullPath, perm)
	if err != nil {
		log.Errorln(err)
	}
}

// Remove a temporary transfer directory and any transferred parts.
func (iom *IOMeshage) destroyTempTransfer(filename string) {
	iom.transferLock.RLock()
	t, ok := iom.transfers[filename]
	iom.transferLock.RUnlock()
	if !ok {
		log.Errorln("could not access transfer object!")
		return
	}

	iom.drainLock.Lock()
	defer iom.drainLock.Unlock()
	err := os.RemoveAll(t.Dir)
	if err != nil {
		log.Errorln(err)
	}
	iom.transferLock.Lock()
	delete(iom.transfers, filename)
	iom.transferLock.Unlock()
}

// Transfer a single filepart to a temporary transfer directory.
func (iom *IOMeshage) Xfer(filename string, part int64, from string) error {
	TID := genTID()
	c := make(chan *IOMMessage)
	err := iom.registerTID(TID, c)
	defer iom.unregisterTID(TID)

	if err != nil {
		// a collision in int64, we should tell someone about this
		log.Fatalln(err)
	}

	m := &IOMMessage{
		From:     iom.node.Name(),
		Type:     TYPE_XFER,
		Filename: filename,
		TID:      TID,
		Part:     part,
	}
	_, err = iom.node.Set([]string{from}, m)
	if err != nil {
		return err
	}

	// wait for a response, or a timeout
	select {
	case resp := <-c:
		if log.WillLog(log.DEBUG) {
			log.Debugln("got part: ", resp.Part)
		}
		if resp.ACK {
			if log.WillLog(log.DEBUG) {
				log.Debugln("got part from: ", resp.From)
			}
			// write the part out to disk
			iom.transferLock.RLock()
			defer iom.transferLock.RUnlock()
			if t, ok := iom.transfers[filename]; ok {
				outfile := fmt.Sprintf("%v/%v.part_%v", t.Dir, filepath.Base(filename), part)
				err := ioutil.WriteFile(outfile, resp.Data, 0664)
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("no transfer temporary directory to write to!")
			}
		} else {
			return fmt.Errorf("received NACK from xfer node")
		}
	case <-time.After(timeout):
		return fmt.Errorf("timeout")
	}
	return nil
}

// Check iom messages that are routing through us in case it's a filepart that
// we're also looking for. If so, write it out. The message mux for meshage
// should call this.
func (iom *IOMeshage) MITM(m *IOMMessage) {
	if m.Type != TYPE_RESPONSE || !m.ACK || len(m.Data) == 0 {
		return
	}

	iom.transferLock.Lock()
	defer iom.transferLock.Unlock()
	if f, ok := iom.transfers[m.Filename]; ok {
		if f.Inflight == m.Part {
			return
		}
		if !f.Parts[m.Part] {
			log.Debug("snooped filepart %v;%v", f.Filename, m.Part)
			outfile := fmt.Sprintf("%v/%v.part_%v", f.Dir, filepath.Base(f.Filename), m.Part)
			err := ioutil.WriteFile(outfile, m.Data, 0664)
			if err != nil {
				log.Errorln(err)
				return
			}
			f.Parts[m.Part] = true
		}
	}
}

// Status returns a deep copy of the in-flight file transfers
func (iom *IOMeshage) Status() []*Transfer {
	iom.transferLock.RLock()
	defer iom.transferLock.RUnlock()

	res := []*Transfer{}

	for _, t := range iom.transfers {
		t2 := new(Transfer)

		// Make shallow copies of all fields
		*t2 = *t

		// Make deep copies
		t2.Parts = make(map[int64]bool)
		for k, v := range t.Parts {
			t2.Parts[k] = v
		}

		res = append(res, t2)
	}

	return res
}

// Delete a file
func (iom *IOMeshage) Delete(file string) error {
	glob, err := filepath.Glob(iom.dirPrep(file))
	if err != nil {
		return err
	}

	for _, v := range glob {
		if v == iom.base {
			// the user *probably* doesn't want to actually remove the iom.base
			// directory since them they wouldn't be able to transfer any more
			// files. Instead, remove all it's contents.
			log.Info("deleting iomeshage directory contents")
			files, err := ioutil.ReadDir(iom.base)
			if err != nil {
				return err
			}

			for _, file := range files {
				if err := os.RemoveAll(filepath.Join(iom.base, file.Name())); err != nil {
					return err
				}
			}

			return nil
		}

		if err := os.RemoveAll(v); err != nil {
			return err
		}
	}

	return nil
}

// Get a full path, with the iom base directory and any trailing "/".
func (iom *IOMeshage) dirPrep(dir string) string {
	// prepend a "/" to the directory so that commands can't affect files above
	// the iom.base directory. For example, filepath.Clean will replace "/../"
	// with "/".
	dir = filepath.Join(iom.base, filepath.Clean("/"+dir))
	log.Info("dir is %v", dir)

	return dir
}

// Generate a random 63 bit TID (positive int64).
func genTID() int64 {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	return r.Int63()
}
