package main

import (
	"io"
	"net"
	"os"
)

func update(path, file string) error {
	dst, err := net.Dial("unix", path)
	if err != nil {
		return err
	}
	defer dst.Close()

	src, err := os.Open(file)
	if err != nil {
		return err
	}
	defer src.Close()

	io.Copy(dst, src)
	return nil
}
