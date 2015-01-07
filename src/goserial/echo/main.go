package main

import (
	"flag"
	"fmt"
	"goserial"
	"log"
	"os"
)

var (
	baud = flag.Int("baud", 115200, "set the baud rate")
)

func usage() {
	fmt.Printf("USAGE: %s [OPTION]... PORT\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(1)
	}

	c := &serial.Config{Name: flag.Arg(0), Baud: *baud}

	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}

	var n int
	buf := make([]byte, 128)
	for {
		n, err = s.Read(buf)
		if err != nil {
			log.Fatal(err)
		}

		_, err := s.Write(buf[:n])
		if err != nil {
			log.Fatal(err)
		}
	}
}
