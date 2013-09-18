// iomeshage is a file transfer layer for meshage
//
// Files are stored in a predetermined directory structure. When a particular
// meshage node needs a file, it polls nodes looking for that file, looking at
// shortest path nodes first. The node with the file and the fewest hops will
// transfer the file to the requesting node. Any nodes along the route will
// also cache the file, unless the node has caching turned off (on by default).
package iomeshage

import (
	"io/ioutil"
	"meshage"
	log "minilog"
	"os"
	"strings"
	"time"
)

// IOMeshage object, which must have a base path to serve files on and a
// meshage node.
type IOMeshage struct {
	base  string        // base path for serving files
	node  *meshage.Node // meshage node to use
	Cache bool          // true if this node should cache files routed through it
}

// FileInfo contains information about a file or directory being served by iomeshage.
type FileInfo struct {
	Name string // name of the file or directory, rooted at IOMeshage.Base
	Dir  bool   // true if the FileInfo is describing a directory
	Size int64  // Size of the file in bytes
}

// Transfer describes an in-flight message.
type Transfer struct {
	File             FileInfo      // Description of the file or directory being transferred
	Duration         time.Duration // How long the file transfer has been running
	BytesTransferred int64         // Number of bytes transferred so far.
}

// New returns a new iomeshage object service base directory b on meshage node
// n, and optionally caching
func New(base string, node *meshage.Node, cache bool) (*IOMeshage, error) {
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	log.Debug("new iomeshage node on base %v", base)
	err := os.MkdirAll(base, 0644)
	return &IOMeshage{
		base:  base,
		node:  node,
		Cache: cache,
	}, err
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
	return nil
}

// Return status on in-flight file transfers
func (iom *IOMeshage) Status() []Transfer {
	return nil
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
