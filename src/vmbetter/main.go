package main

import (
	"vmconfig"
	"fmt"
)

func main() {
	m, err := vmconfig.ReadConfig("test")
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(m)
	}
}
