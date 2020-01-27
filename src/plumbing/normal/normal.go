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
	f_seed   = flag.Int64("seed", -1, "use seed value (for testing purposes)")
)

func main() {
	flag.Parse()

	var s rand.Source
	if *f_seed != -1 {
		s = rand.NewSource(*f_seed)
	} else {
		s = rand.NewSource(time.Now().UnixNano())
	}

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
