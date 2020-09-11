package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	count := 0

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		count++
		fmt.Fprintf(os.Stdout, "%v\n", count)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
