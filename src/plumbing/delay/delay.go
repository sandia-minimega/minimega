package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"time"
)

var (
	f_delay = flag.String("t", "1.0s", "delay")
)

func main() {
	flag.Parse()

	d, err := time.ParseDuration(*f_delay)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		time.Sleep(d)
		fmt.Fprintf(os.Stdout, "%v\n", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
