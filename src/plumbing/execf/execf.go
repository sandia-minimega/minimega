package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "argument mismatch!")
		return
	}

	argf := os.Args[1]

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		args := fieldsQuoteEscape("\"", fmt.Sprintf(argf, scanner.Text()))
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			fmt.Fprintln(os.Stderr, string(out))
			fmt.Fprintln(os.Stderr, err)
			return
		}
		fmt.Fprintf(os.Stdout, string(out))
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Return a slice of strings, split on whitespace, not unlike strings.Fields(),
// except that quoted fields are grouped.
// 	Example: a b "c d"
// 	will return: ["a", "b", "c d"]
func fieldsQuoteEscape(c string, input string) []string {
	f := strings.Fields(input)
	var ret []string
	trace := false
	temp := ""

	for _, v := range f {
		if trace {
			if strings.Contains(v, c) {
				trace = false
				temp += " " + trimQuote(c, v)
				ret = append(ret, temp)
			} else {
				temp += " " + v
			}
		} else if strings.Contains(v, c) {
			temp = trimQuote(c, v)
			if strings.HasSuffix(v, c) {
				// special case, single word like 'foo'
				ret = append(ret, temp)
			} else {
				trace = true
			}
		} else {
			ret = append(ret, v)
		}
	}
	return ret
}

func trimQuote(c string, input string) string {
	if c == "" {
		return ""
	}
	var ret string
	for _, v := range input {
		if v != rune(c[0]) {
			ret += string(v)
		}
	}
	return ret
}
