// iomeshage is a file transfer layer for meshage
//
// Files are stored in a predetermined directory structure. When a particular
// meshage node needs a file, it polls nodes looking for that file, looking at
// shortest path nodes first. The node with the file and the fewest hops will
// transfer the file to the requesting node. Any nodes along the route will
// also cache the file, unless the node has caching turned off (on by default).
package iomeshage

import (
	"meshage"
	"time"
)

// IOMeshage object, which must have a base path to serve files on and a
// meshage node.
type IOMeshage struct {
	Base  string        // base path for serving files
	Node  *Meshage.Node // meshage node to use
	Cache bool          // true if this node should cache files routed through it
}

// FileInfo contains information about a file or directory being served by iomeshage.
type FileInfo struct {
	Name string // name of the file or directory, rooted at IOMeshage.Base
	Dir  bool   // true if the FileInfo is describing a directory
	Size int64  // Size of the file in bytes
	Hash string // sha1 hash of the file
}

// Transfer describes an in-flight message.
type Transfer struct {
	File             FileInfo      // Description of the file or directory being transferred
	Duration         time.Duration // How long the file transfer has been running
	BytesTransferred int64         // Number of bytes transferred so far.
}

// List files and directories starting at iom.Base+dir
func (iom *IOMeshage) List(dir string) []FileInfo {

}

// Retrieve a file from the shortest path node that has it. Get blocks until
// the file transfer is complete or errors out. If the file specified is a
// directory, the entire directory will be recursively transferred.
func (iom *IOMeshage) Get(file string) error {

}

// Retrieve a file from the shortest path node that has it, and return control
// immediately. In-flight transfers can be checked with Status(). If the file
// specified is a directory, the entire directory will be recursively
// tranferred.
func (iom *IOMeshage) GetNB(file string) {

}

// Return status on in-flight file transfers
func (iom *IOMeshage) Status() []Transfer {

}

// Delete a file
func (iom *IOMeshage) Delete(file string) error {

}
