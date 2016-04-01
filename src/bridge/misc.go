// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

// DestroyBridge deletes an `unmanaged` bridge. This can be used when cleaning
// up from a crash. See `Bride.Destroy` for managed bridges.
func DestroyBridge(b string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return ovsDelBridge(b)
}
