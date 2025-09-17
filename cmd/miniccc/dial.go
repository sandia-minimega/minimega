package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Retry to connect for 120 minutes, fail after that
const Retries = 480
const RetryInterval = 15 * time.Second

var errTimeout = fmt.Errorf("timeout waiting for function")

func dial() error {
	client.Lock()
	defer client.Unlock()

	var err error

	for i := Retries; i > 0; i-- {
		if *f_serial == "" {
			log.Debug("dial: %v:%v:%v", *f_family, *f_parent, *f_port)

			var addr string
			switch *f_family {
			case "tcp":
				addr = fmt.Sprintf("%v:%v", *f_parent, *f_port)
			case "unix":
				addr = *f_parent
			default:
				log.Fatal("invalid ron dial network family: %v", *f_family)
			}

			client.conn, err = net.Dial(*f_family, addr)
		} else {
			err = timeout(ron.CLIENT_RECONNECT_RATE*time.Second, func() (err error) {
				client.conn, err = dialSerial(*f_serial)
				if err != nil {
					err = fmt.Errorf("dialing serial port: %v", err)
				}

				return
			})
		}

		// Handle any errors with client initialization before we attempt to read 
		// from the client below. Attempt another connection if there's any errors.
		if err != nil {
			log.Error("%v, retries = %v", err, i)
			time.Sleep(15 * time.Second)
			continue
		}

		// write magic bytes
		_, err = io.WriteString(client.conn, "RON")

		err = timeout(ron.CLIENT_RECONNECT_RATE*time.Second, func() (err error) {
			// read until we see the magic bytes back
			var buf [3]byte
			for err == nil && string(buf[:]) != "RON" {
				// shift the buffer
				buf[0] = buf[1]
				buf[1] = buf[2]
				// read the next byte
				_, err = client.conn.Read(buf[2:])
			}

			if err != nil {
				err = fmt.Errorf("reading magic bytes from ron: %v", err)
			}

			return
		})

		if err == nil {
			client.enc = gob.NewEncoder(client.conn)
			client.dec = gob.NewDecoder(client.conn)
			return nil
		}

		log.Error("%v, retries = %v", err, i)

		// It's possible that we could have an error after the client connection has
		// been created. For example, when using the serial port, writing the magic
		// `RON` bytes can result in an EOF if the host has been rebooted and the
		// minimega server hasn't cleaned up and reconnected to the virtual serial
		// port yet. In such a case, the connection needs to be closed to avoid a
		// "device busy" error when trying to dial it again.
		if client.conn != nil {
			client.conn.Close()
		}

		time.Sleep(15 * time.Second)
	}

	return err
}

func timeout(d time.Duration, f func() error) error {
	c := make(chan error)

	go func() {
		c <- f()
	}()

	select {
	case err := <-c:
		return err
	case <-time.After(d):
		return errTimeout
	}
}
