package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Retry to connect for 120 minutes, fail after that
const Retries = 480

// RetryInterval is the longest wait between two connection attempts.
const RetryInterval = 15 * time.Second

// firstRetryInterval is the wait before the first retry. Each later wait is
// twice as long, up to RetryInterval.
var firstRetryInterval = time.Second

var errTimeout = fmt.Errorf("timeout waiting for function")

var errVsockUnsupported = errors.New("-family vsock is only supported on Linux")

var errNotConnected = errors.New("not connected to the cc server")

// vsockDialer dials the host for -family vsock. Tests replace it.
var vsockDialer = dialVsock

func dial() error {
	client.Lock()
	defer client.Unlock()

	client.acked = false

	var err error
	wait := firstRetryInterval

	for i := Retries; i > 0; i-- {
		var conn io.ReadWriteCloser

		if *f_serial == "" {
			log.Debug("dial: %v:%v:%v", *f_family, *f_parent, *f_port)

			var addr string
			switch *f_family {
			case "tcp":
				addr = fmt.Sprintf("%v:%v", *f_parent, *f_port)
			case "unix":
				addr = *f_parent
			case "vsock":
			default:
				log.Fatal("invalid ron dial network family: %v", *f_family)
			}

			if *f_family == "vsock" {
				conn, err = vsockDialer(*f_port)
				if errors.Is(err, errVsockUnsupported) {
					log.Fatal("%v", err)
				}
			} else {
				conn, err = net.Dial(*f_family, addr)
			}
		} else {
			var serial io.ReadWriteCloser
			err = timeout(ron.CLIENT_RECONNECT_RATE*time.Second, func() (err error) {
				serial, err = dialSerial(*f_serial)
				if err != nil {
					err = fmt.Errorf("dialing serial port: %v", err)
				}

				return
			})
			if err == nil {
				conn = serial
			}
		}

		// Handle any errors with client initialization before we attempt to read
		// from the client below. Attempt another connection if there's any errors.
		if err != nil {
			log.Error("%v, retries = %v", err, i)
			wait = sleepRetry(wait)
			continue
		}

		// write magic bytes
		if _, err = io.WriteString(conn, "RON"); err != nil {
			err = fmt.Errorf("writing magic bytes to ron: %v", err)
		} else {
			err = timeout(ron.CLIENT_RECONNECT_RATE*time.Second, func() (err error) {
				// read until we see the magic bytes back
				var buf [3]byte
				for err == nil && string(buf[:]) != "RON" {
					// shift the buffer
					buf[0] = buf[1]
					buf[1] = buf[2]
					// read the next byte
					_, err = conn.Read(buf[2:])
				}

				return
			})
			if err != nil {
				err = fmt.Errorf("reading magic bytes from ron: %v", err)
			}
		}

		if err == nil {
			client.conn = conn
			client.dec = gob.NewDecoder(conn)

			client.writeMu.Lock()
			client.enc = gob.NewEncoder(conn)
			client.lost = make(chan error, 1)
			client.writeMu.Unlock()

			return nil
		}

		log.Error("%v, retries = %v", err, i)

		// It's possible that we could have an error after the client connection has
		// been created. For example, when using the serial port, writing the magic
		// `RON` bytes can result in an EOF if the host has been rebooted and the
		// minimega server hasn't cleaned up and reconnected to the virtual serial
		// port yet. In such a case, the connection needs to be closed to avoid a
		// "device busy" error when trying to dial it again.
		conn.Close()

		wait = sleepRetry(wait)
	}

	return err
}

// sleepRetry sleeps for wait and returns the wait before the next retry.
func sleepRetry(wait time.Duration) time.Duration {
	time.Sleep(wait)

	return nextRetry(wait)
}

// nextRetry returns the wait that follows wait.
func nextRetry(wait time.Duration) time.Duration {
	return min(2*wait, RetryInterval)
}

func timeout(d time.Duration, f func() error) error {
	// buffered, so that f can return after the timeout
	c := make(chan error, 1)

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
