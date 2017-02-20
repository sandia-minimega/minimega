package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

var (
	f_stddev = flag.Float64("stddev", 0.0, "standard deviation")
)

func main() {
	flag.Parse()

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		f, err := strconv.ParseFloat(scanner.Text(), 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		fout := r.NormFloat64()**f_stddev + f
		fmt.Println(fout)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
