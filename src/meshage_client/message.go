package main

import (
	"fmt"
	"meshage"
)

func messageHandler(m chan meshage.Message) {
	for {
		message := <-m
		var mtype string
		if message.MessageType == meshage.BROADCAST {
			mtype = "broadcast"
		} else {
			mtype = "set"
		}
		fmt.Printf("\ngot %s message from %s via %v : %s\n", mtype, message.Source, message.CurrentRoute, message.Body.(string))
	}
}
