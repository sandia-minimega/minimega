package main

import (
	"fmt"
	"meshage"
)

func messageHandler(m chan meshage.Message) {
	for {
		message := <-m
		fmt.Printf("got message from %s via %v : %s\n", message.Source, message.CurrentRoute, message.Body.(string))
		if message.Body.(string) == "ping" {
			n.Send([]string{message.Source}, "pong")
		}
	}
}
