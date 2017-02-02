// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "minilog"
)

func pipeHandler(pipe string) {
	host := filepath.Join(*f_path, "miniccc")

	conn, err := net.Dial("unix", host)
	if err != nil {
		return err
	}

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	err = enc.Encode(MODE_PIPE)
	if err != nil {
		log.Fatalln(err)
	}

	// encode the pipe name
	err = enc.Encode(pipe)
	if err != nil {
		log.Fatalln(err)
	}

	// from here we just encode/decode on the pipe

	go func() {
		var buf string
		for {
			err := dec.Decode(&buf)
			if err != nil {
				if err != io.EOF {
					log.Fatal("local command gob decode: %v", err)
				}
				os.Exit(0)
			}

			_, err = os.Stdout.WriteString(buf)
			if err != nil {
				log.Fatal("write: %v", err)
			}
		}
	}()

	for {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			err := enc.Encode(scanner.Text() + "\n")
			if err != nil {
				log.Fatal("local command gob encode: %v", err)
			}
		}

		// scanners don't return EOF errors
		if err := scanner.Err(); err != nil {
			log.Fatal("read: %v", err)
		}

		log.Debugln("client closed stdin")
		os.Exit(0)
	}
}
