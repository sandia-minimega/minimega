package main

import (
	"fmt"
	"telnet"
	"log"
	"os"
	"time"
)

const timeout = 10 * time.Second

func checkErr(err error) {
	if err != nil {
		log.Fatalln("Error:", err)
	}
}

func expect(t *telnet.Conn, d ...string) {
	checkErr(t.SetReadDeadline(time.Now().Add(timeout)))
	checkErr(t.SkipUntil(d...))
}

func sendln(t *telnet.Conn, s string) {
	checkErr(t.SetWriteDeadline(time.Now().Add(timeout)))
	buf := make([]byte, len(s)+1)
	copy(buf, s)
	buf[len(s)] = '\n'
	_, err := t.Write(buf)
	checkErr(err)
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s USER PASSWD\n", os.Args[0])
		return
	}
	user, passwd := os.Args[1], os.Args[2]

	t, err := telnet.Dial("tcp", "127.0.0.1:23")
	checkErr(err)
	t.SetUnixWriteMode(true)

	expect(t, "login: ")
	sendln(t, user)
	expect(t, "ssword: ")
	sendln(t, passwd)
	expect(t, "$")
	sendln(t, "ls -l")

	ls, err := t.ReadBytes('$')
	checkErr(err)

	os.Stdout.Write(ls)
}
