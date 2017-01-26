// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goftp/server"
)

type FileDriver struct {
	RootPath string
	server.Perm
}

type FileInfo struct {
	os.FileInfo

	mode  os.FileMode
	owner string
	group string
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
	basepath := driver.realPath(path)
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
	return &FileInfo{f, mode, owner, group}, nil
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
	rPath := driver.realPath(path)
	f, err := os.Open(rPath)
	if err != nil {
		return 0, nil, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, nil, err
	}

	f.Seek(offset, os.SEEK_SET)

	return info.Size(), f, nil
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
