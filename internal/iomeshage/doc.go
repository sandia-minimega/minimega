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
