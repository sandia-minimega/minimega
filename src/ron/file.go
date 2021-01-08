// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"errors"
	"io"
	log "minilog"
	"os"
	"path/filepath"
)

// File sent from server to client or client to server. The ID and name
// uniquely identify the transfer. Perm is only used for the first chunk. EOF
// signals that this is the last data chunk.
type File struct {
	ID   int         // command that requested the file
	Name string      // name of the file
	Perm os.FileMode // permissions

	Data   []byte // data chunk
	Offset int64  // offset for this chunk
	EOF    bool   // final chunk in file
}

// Recv part of a file, writing it to <fpath>.partial. Once the last piece of
// the file has been received, renames to remove .partial suffix.
func (f *File) Recv(fpath string) error {
	finalPath := fpath

	// append suffix if this is only part of the file
	if !f.EOF || f.Offset > 0 {
		fpath += ".partial"
	}

	if err := f.Write(fpath); err != nil {
		return err
	}

	// finished writing all the parts so remove suffix
	if f.EOF && f.Offset > 0 {
		return os.Rename(fpath, finalPath)
	}

	return nil
}

// Write file data to fpath at the appropriate permissions and offset. Creates
// the parent directory if needed.
func (f *File) Write(fpath string) error {
	dir := filepath.Dir(fpath)

	if err := os.MkdirAll(dir, os.FileMode(0770)); err != nil {
		return err
	}

	file, err := os.OpenFile(fpath, os.O_RDWR|os.O_CREATE, f.Perm)
	if err != nil {
		return err
	}
	defer file.Close()

	n, err := file.WriteAt(f.Data, f.Offset)
	if n < len(f.Data) {
		log.Error("partial write of file %v, check disk", f.Name)
	}
	return err
}

// SendFile sends a file in chunks using the send func.
func SendFile(dir, fpath string, ID int, chunkSize int64, send func(m *Message) error) error {
	rel, err := filepath.Rel(dir, fpath)
	if err != nil {
		return err
	}

	sendError := func(err error) error {
		return send(&Message{
			Type: MESSAGE_FILE,
			File: &File{
				ID:   ID,
				Name: rel,
			},
			Error: err.Error(),
		})
	}

	f, err := os.Open(fpath)
	if err != nil {
		log.Error("cannot open file %v: %v", fpath, err)
		return sendError(err)
	}
	defer f.Close()

	// we do have the file, calculate the number of parts
	fi, err := f.Stat()
	if err != nil {
		log.Error("cannot stat file %v: %v", fpath, err)
		return sendError(err)
	}

	if fi.IsDir() {
		// can't send directory
		return sendError(errors.New("cannot send directory"))
	}

	var offset int64

	for {
		data := make([]byte, chunkSize)

		n, err := f.Read(data)
		if err != nil && err != io.EOF {
			log.Error("cannot read file %v: %v", fpath, err)
			return sendError(err)
		}

		m := &Message{
			Type: MESSAGE_FILE,
			File: &File{
				ID:     ID,
				Name:   rel,
				Perm:   fi.Mode() & os.ModePerm,
				Data:   data[:n],
				EOF:    err == io.EOF,
				Offset: offset,
			},
		}

		offset += int64(n)

		if err := send(m); err != nil {
			return err
		}

		if err == io.EOF {
			break
		}
	}

	return nil
}
