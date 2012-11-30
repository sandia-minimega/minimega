package main

import (
	"fmt"
	"meshage"
)

func messageHandler(m chan meshage.Message) {
	for {
		message := <-m
		fmt.Printf("\ngot message from %s via %v : %s\n", message.Source, message.CurrentRoute, message.Body.(string))
	}
}
