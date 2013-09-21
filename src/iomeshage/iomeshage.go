// iomeshage is a file transfer layer for meshage
//
// Files are stored in a predetermined directory structure. When a particular
// meshage node needs a file, it polls nodes looking for that file, looking at
// shortest path nodes first. The node with the file and the fewest hops will
// transfer the file to the requesting node. Any nodes along the route will
// also cache the file, unless the node has caching turned off (on by default).
package iomeshage

// TODO: local check to see if the file exists
// TODO: caching

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"meshage"
	log "minilog"
	"os"
	"strings"
	"sync"
	"time"
)

// IOMeshage object, which must have a base path to serve files on and a
// meshage node.
type IOMeshage struct {
	base      string                // base path for serving files
	node      *meshage.Node         // meshage node to use
	Cache     bool                  // true if this node should cache files routed through it
	Messages  chan *meshage.Message // Incoming messages from meshage
	TIDs      map[int64]chan *IOMMessage
	transfers map[string]*Transfer // current transfers or caches of file parts
	drainLock sync.RWMutex
}

type FileInfo struct {
	Name string
	Dir  bool
	Size int64
}

// Transfer describes an in-flight transfer or cache of file parts
type Transfer struct {
	Dir      string         // temporary directory hold the file parts
	Filename string         // file name
	Parts    map[int64]bool // completed parts
	NumParts int
}

var (
	timeout = time.Duration(30 * time.Second)
)

// New returns a new iomeshage object service base directory b on meshage node
// n, and optionally caching
func New(base string, node *meshage.Node, cache bool) (*IOMeshage, error) {
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	log.Debug("new iomeshage node on base %v", base)
	err := os.MkdirAll(base, 0755)

	r := &IOMeshage{
		base:      base,
		node:      node,
		Cache:     cache,
		Messages:  make(chan *meshage.Message, 1024),
		TIDs:      make(map[int64]chan *IOMMessage),
		transfers: make(map[string]*Transfer),
	}

	go r.handleMessages()

	return r, err
}

// List files and directories starting at iom.Base+dir
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

// Retrieve a file from the shortest path node that has it. Get blocks until
// the file transfer is begins or errors out. If the file specified is a
// directory, the entire directory will be recursively transferred.
// If the file already exists on this node, Get will return immediately with no
// error.
func (iom *IOMeshage) Get(file string) error {
	// is this file available locally?
	_, err := iom.fileInfo(file)
	if err == nil {
		return nil
	}

	// is this file already in flight?
	if _, ok := iom.transfers[file]; ok {
		return fmt.Errorf("file already in flight")
	}

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
	n, err := iom.node.Broadcast(meshage.UNORDERED, m)
	if err != nil {
		return err
	}
	log.Debug("sent info request to %v nodes", n)

	var info *IOMMessage
	var gotInfo bool
	// wait for n responses, or a timeout
	for i := 0; i < n; i++ {
		select {
		case resp := <-c:
			log.Debugln("got response: ", resp)
			if resp.ACK {
				log.Debugln("got info from: ", resp.From)
				info = resp
				gotInfo = true
			}
		case <-time.After(timeout):
			return fmt.Errorf("timeout")
		}
	}
	if !gotInfo {
		return fmt.Errorf("file not found")
	}

	log.Debug("found file on node %v with %v parts", info.From, info.Part)

	go iom.getParts(info.Filename, info.Part)

	return nil
}

func (iom *IOMeshage) getParts(filename string, numParts int64) {
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

	// now get the parts
	TID := genTID()
	c := make(chan *IOMMessage)
	err := iom.registerTID(TID, c)
	defer iom.unregisterTID(TID)

	if err != nil {
		// a collision in int64, we should tell someone about this
		log.Fatalln(err)
	}

	// create a transfer object
	tdir, err := ioutil.TempDir(iom.base, "transfer_")
	if err != nil {
		log.Errorln(err)
		return
	}
	iom.transfers[filename] = &Transfer{
		Dir:      tdir,
		Filename: filename,
		Parts:    make(map[int64]bool),
		NumParts: len(parts),
	}
	defer iom.destroyTempTransfer(filename)

	for _, p := range parts {
		m := &IOMMessage{
			From:     iom.node.Name(),
			Type:     TYPE_WHOHAS,
			Filename: filename,
			TID:      TID,
			Part:     p,
		}
		n, err := iom.node.Broadcast(meshage.UNORDERED, m)
		if err != nil {
			log.Errorln(err)
			return
		}
		log.Debug("sent info request to %v nodes", n)

		var info *IOMMessage
		var gotPart bool
		// wait for n responses, or a timeout
		for i := 0; i < n; i++ {
			select {
			case resp := <-c:
				log.Debugln("got response: ", resp)
				if resp.ACK {
					log.Debugln("got partInfo from: ", resp.From)
					info = resp
					gotPart = true
				}
			case <-time.After(timeout):
				log.Errorln("timeout")
				return
			}
		}
		if !gotPart {
			log.Errorln("part not found")
			return
		}

		log.Debug("found part %v on node %v", info.Part, info.From)

		// transfer this part
		err = iom.Xfer(m.Filename, info.Part, info.From)
		if err != nil {
			log.Errorln(err)
			return
		}
		iom.transfers[filename].Parts[p] = true
	}

	// copy the parts into the whole file
	t := iom.transfers[filename]
	tfile, err := ioutil.TempFile(t.Dir, "cat_")
	if err != nil {
		log.Errorln(err)
	}

	for i = 0; i < numParts; i++ {
		fname := fmt.Sprintf("%v/%v.part_%v", t.Dir, filename, i)
		fpart, err := os.Open(fname)
		if err != nil {
			log.Errorln(err)
			return
		}
		io.Copy(tfile, fpart)
		fpart.Close()
	}
	name := tfile.Name()
	tfile.Close()
	os.Rename(name, iom.base+filename)
}

func (iom *IOMeshage) destroyTempTransfer(filename string) {
	t, ok := iom.transfers[filename]
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
	delete(iom.transfers, filename)
}

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
	err = iom.node.Set([]string{from}, meshage.UNORDERED, m)
	if err != nil {
		return err
	}

	// wait for a response, or a timeout
	select {
	case resp := <-c:
		log.Debugln("got part: ", resp.Part)
		if resp.ACK {
			log.Debugln("got part from: ", resp.From)
			// write the part out to disk
			if t, ok := iom.transfers[filename]; ok {
				outfile := fmt.Sprintf("%v/%v.part_%v", t.Dir, filename, part)
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

// Return status on in-flight file transfers
func (iom *IOMeshage) Status() []*Transfer {
	var ret []*Transfer
	for _, t := range iom.transfers {
		ret = append(ret, t)
	}
	return ret
}

// Delete a file
func (iom *IOMeshage) Delete(file string) error {
	file = iom.dirPrep(file)
	return os.RemoveAll(file)
}

func (iom *IOMeshage) dirPrep(dir string) string {
	if strings.HasPrefix(dir, "/") {
		dir = strings.TrimLeft(dir, "/")
	}
	log.Debug("dir is %v%v\n", iom.base, dir)
	return iom.base + dir
}

func genTID() int64 {
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	return r.Int63()
}
