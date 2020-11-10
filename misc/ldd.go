package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	p := os.Args[1]
	d := os.Args[2]
	o, err := exec.Command("ldd", p).Output()
	if err != nil {
		fmt.Println(err)
		return
	}
	b := bytes.NewBuffer(o)

	s := bufio.NewScanner(b)
	for s.Scan() {
		f := strings.Fields(s.Text())
		var l string
		if len(f) == 2 || len(f) == 3 {
			l = f[0]
			if filepath.Dir(l) == "." {
				continue
			}
		} else {
			l = f[2]
		}
		dst := filepath.Join(d, filepath.Dir(l))
		err := os.MkdirAll(dst, 0755)
		if err != nil {
			fmt.Println(err)
			return
		}
		err = exec.Command("cp", l, dst).Run()
		if err != nil {
			fmt.Println(err)
			return
		}
	}
	if err := s.Err(); err != nil {
		fmt.Println(err)
		return
	}
}
