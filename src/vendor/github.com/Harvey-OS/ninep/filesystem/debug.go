// Copyright 2009 The Ninep Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ufs

import (
	"bytes"

	"github.com/Harvey-OS/ninep/protocol"
)

type debugFileServer struct {
	*FileServer
}

func (e *debugFileServer) Rversion(msize protocol.MaxSize, version string) (protocol.MaxSize, string, error) {
	e.logf(">>> Tversion %v %v\n", msize, version)
	msize, version, err := e.FileServer.Rversion(msize, version)
	if err == nil {
		e.logf("<<< Rversion %v %v\n", msize, version)
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return msize, version, err
}

func (e *debugFileServer) Rattach(fid protocol.FID, afid protocol.FID, uname string, aname string) (protocol.QID, error) {
	e.logf(">>> Tattach fid %v,  afid %v, uname %v, aname %v\n", fid, afid,
		uname, aname)
	qid, err := e.FileServer.Rattach(fid, afid, uname, aname)
	if err == nil {
		e.logf("<<< Rattach %v\n", qid)
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return qid, err
}

func (e *debugFileServer) Rflush(o protocol.Tag) error {
	e.logf(">>> Tflush tag %v\n", o)
	err := e.FileServer.Rflush(o)
	if err == nil {
		e.logf("<<< Rflush\n")
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return err
}

func (e *debugFileServer) Rwalk(fid protocol.FID, newfid protocol.FID, paths []string) ([]protocol.QID, error) {
	e.logf(">>> Twalk fid %v, newfid %v, paths %v\n", fid, newfid, paths)
	qid, err := e.FileServer.Rwalk(fid, newfid, paths)
	if err == nil {
		e.logf("<<< Rwalk %v\n", qid)
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return qid, err
}

func (e *debugFileServer) Ropen(fid protocol.FID, mode protocol.Mode) (protocol.QID, protocol.MaxSize, error) {
	e.logf(">>> Topen fid %v, mode %v\n", fid, mode)
	qid, iounit, err := e.FileServer.Ropen(fid, mode)
	if err == nil {
		e.logf("<<< Ropen %v %v\n", qid, iounit)
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return qid, iounit, err
}

func (e *debugFileServer) Rcreate(fid protocol.FID, name string, perm protocol.Perm, mode protocol.Mode) (protocol.QID, protocol.MaxSize, error) {
	e.logf(">>> Tcreate fid %v, name %v, perm %v, mode %v\n", fid, name,
		perm, mode)
	qid, iounit, err := e.FileServer.Rcreate(fid, name, perm, mode)
	if err == nil {
		e.logf("<<< Rcreate %v %v\n", qid, iounit)
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return qid, iounit, err
}

func (e *debugFileServer) Rclunk(fid protocol.FID) error {
	e.logf(">>> Tclunk fid %v\n", fid)
	err := e.FileServer.Rclunk(fid)
	if err == nil {
		e.logf("<<< Rclunk\n")
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return err
}

func (e *debugFileServer) Rstat(fid protocol.FID) ([]byte, error) {
	e.logf(">>> Tstat fid %v\n", fid)
	b, err := e.FileServer.Rstat(fid)
	if err == nil {
		dir, _ := protocol.Unmarshaldir(bytes.NewBuffer(b))
		e.logf("<<< Rstat %v\n", dir)
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return b, err
}

func (e *debugFileServer) Rwstat(fid protocol.FID, b []byte) error {
	dir, _ := protocol.Unmarshaldir(bytes.NewBuffer(b))
	e.logf(">>> Twstat fid %v, %v\n", fid, dir)
	err := e.FileServer.Rwstat(fid, b)
	if err == nil {
		e.logf("<<< Rwstat\n")
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return err
}

func (e *debugFileServer) Rremove(fid protocol.FID) error {
	e.logf(">>> Tremove fid %v\n", fid)
	err := e.FileServer.Rremove(fid)
	if err == nil {
		e.logf("<<< Rremove\n")
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return err
}

func (e *debugFileServer) Rread(fid protocol.FID, o protocol.Offset, c protocol.Count) ([]byte, error) {
	e.logf(">>> Tread fid %v, off %v, count %v\n", fid, o, c)
	b, err := e.FileServer.Rread(fid, o, c)
	if err == nil {
		e.logf("<<< Rread %v\n", len(b))
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return b, err
}

func (e *debugFileServer) Rwrite(fid protocol.FID, o protocol.Offset, b []byte) (protocol.Count, error) {
	e.logf(">>> Twrite fid %v, off %v, count %v\n", fid, o, len(b))
	c, err := e.FileServer.Rwrite(fid, o, b)
	if err == nil {
		e.logf("<<< Rwrite %v\n", c)
	} else {
		e.logf("<<< Error %v\n", err)
	}
	return c, err
}
