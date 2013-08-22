package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"ranges"
	"strings"
)

var (
	f_prefix = flag.String("prefix", "kn", "prefix to use when ranging")
)

func main() {
	flag.Parse()

	r, _ := ranges.NewRange(*f_prefix, 1, 520)

	data, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
	input := strings.Fields(string(data))

	res, err := r.UnsplitRange(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println(res)
}
