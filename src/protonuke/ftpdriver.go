// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goftp/server"
)

type ReadCloser struct {
	*bytes.Reader
}

type FileDriver struct {
	RootPath string
	server.Perm
}

type FileInfo struct {
	name  string
	size  int64
	dir   bool
	mode  os.FileMode
	owner string
	group string
}

func (r *ReadCloser) Close() error {
	//noop
	return nil
}

func (f *FileInfo) Name() string {
	return f.name
}

func (f *FileInfo) Size() int64 {
	return int64(f.size)
}

func (f *FileInfo) Mode() os.FileMode {
	return f.mode
}

func (f *FileInfo) Owner() string {
	return f.owner
}

func (f *FileInfo) Group() string {
	return f.group
}

func (f *FileInfo) ModTime() time.Time {
	return time.Now()
}

func (f *FileInfo) IsDir() bool {
	return f.dir
}

func (f *FileInfo) Sys() interface{} {
	return nil
}

func (driver *FileDriver) realPath(path string) string {
	paths := strings.Split(path, "/")
	return filepath.Join(append([]string{driver.RootPath}, paths...)...)
}

func (driver *FileDriver) Init(conn *server.Conn) {
	//driver.conn = conn
}

func (driver *FileDriver) ChangeDir(path string) error {
	// noop
	return nil
}

func (driver *FileDriver) Stat(path string) (server.FileInfo, error) {
	mockFileInfo := &FileInfo{
		name:  "ftpFile",
		mode:  0666,
		owner: "protonuke",
		group: "protonuke",
		size:  int64(len(FTPImage)),
		dir:   true,
	}

	return mockFileInfo, nil

	/*basepath := driver.realPath(path)
	rPath, err := filepath.Abs(basepath)
	if err != nil {
		return nil, err
	}
	f, err := os.Lstat(rPath)
	if err != nil {
		return nil, err
	}
	mode, err := driver.Perm.GetMode(path)
	if err != nil {
		return nil, err
	}
	if f.IsDir() {
		mode |= os.ModeDir
	}
	owner, err := driver.Perm.GetOwner(path)
	if err != nil {
		return nil, err
	}
	group, err := driver.Perm.GetGroup(path)
	if err != nil {
		return nil, err
	}
	return &FileInfo{f, mode, owner, group}, nil*/
}

func (driver *FileDriver) ListDir(path string, callback func(server.FileInfo) error) error {
	fileInfo, err := driver.Stat(path)
	if err != nil {
		return err
	}
	return callback(fileInfo)
}

func (driver *FileDriver) DeleteDir(path string) error {
	// noop
	return nil
}

func (driver *FileDriver) DeleteFile(path string) error {
	// noop
	return nil
}

func (driver *FileDriver) Rename(fromPath string, toPath string) error {
	// noop
	return nil
}

func (driver *FileDriver) MakeDir(path string) error {
	// noop
	return nil
}

func (driver *FileDriver) GetFile(path string, offset int64) (int64, io.ReadCloser, error) {
	reader := bytes.NewReader(FTPImage)
	r := &ReadCloser{
		reader,
	}
	r.Seek(offset, os.SEEK_SET)
	return int64(r.Len()), r, nil
}

func (driver *FileDriver) PutFile(destPath string, data io.Reader, appendData bool) (int64, error) {
	// noop
	return 0, nil
}

type FileDriverFactory struct {
	RootPath string
	server.Perm
}

func (factory *FileDriverFactory) NewDriver() (server.Driver, error) {
	return &FileDriver{factory.RootPath, factory.Perm}, nil
}
