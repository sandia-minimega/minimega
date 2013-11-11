package main

import (
	"fmt"
	"io/ioutil"
	"strings"
)

func main() {
	d, err := ioutil.ReadFile("/proc/cmdline")
	if err != nil {
		fmt.Println(err)
		return
	}

	f := strings.Fields(string(d))
	on := false
	var ret string
	for _, v := range f {
		if v == "protonuke" {
			on = true
		} else if v == "endprotonuke" {
			fmt.Println(ret)
			return
		} else if on {
			ret += v + " "
		}
	}
}
