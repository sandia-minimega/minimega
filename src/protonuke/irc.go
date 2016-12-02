// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"math/rand"
	log "minilog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thoj/go-ircevent"
	"github.com/stargrave/goircd"
)

const (
	greeting = "yo"
)

var channels = defaultChannels
var messages = defaultMessages
var nicks    = defaultNicks

func ircClient(protocol string) {
	port := *f_ircport
	if *f_ircchans != "" {
		channels = strings.Split(*f_ircchans, ",")
	}

	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	log.Debugln("ircClient")
	rand.Seed(time.Now().UnixNano())

	host, original := randomHost()

	nick := randomNick()
	client := irc.IRC(nick, nick)

	// generate list of channels to join
	n := 1
	if len(channels) > 1 {
		n += rand.Intn(len(channels) - 1)
	}
	userChannels := []string{}
	for i := 0; i < n; i++ {
		channel := randomChannel()
		found := false
		for _, item := range userChannels {
			if item == channel {
				found = true
				break
			}
		}
		if !found {
			userChannels = append(userChannels, channel)
		}
	}
	joinedChannels := []string{}
	joinedLock := sync.Mutex

	// create callbacks
	// 001: RPL_WELCOME "Welcome to the Internet Relay Network <nick>!<user>@<host>"
	client.AddCallback("001", func(event *irc.Event) {
		go func(event *irc.Event) {
			// join channels
			for i := 0; i < len(userChannels); i++ {
				t.Tick()
				log.Debug("[nick %v] joining channel %v on host %v", client.GetNick(), userChannels[i], host)
				client.Join(userChannels[i])
			}

			for {
				t.Tick()
				if len(joinedChannels) == 0 {
					continue
				}

				to, message := randomMessage(joinedChannels)
				log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), to, message)
				client.Privmsg(to, message)
			}
		}(event)
	})

	// 433: ERR_NICKNAMEINUSE "<nick> :Nickname is already in use"
	client.AddCallback("433", func(event *irc.Event) {
		// Note:  removed race condition from go-ircevent where go-ircevent will 
		//        receive the callback first and append underscores to the name instead

		// nick is taken, attempt to take another nick with a random number
		newNick := nick + strconv.Itoa(rand.Intn(1000000))
		log.Debug("[nick %v] Switching nick to %v", client.GetNick(), newNick)
		client.Nick(newNick)
	})

	// JOIN occurs after you successfully join a channel
	client.AddCallback("JOIN", func(event *irc.Event) {
		if event.Nick == client.GetNick() {
			joinedLock.Lock()
			defer joinedLock.Unlock()

			// add channel to joined channel slice
			joinedChannels = append(joinedChannels, event.Arguments[0])

			// send greeting to channel
			log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), event.Arguments[0], greeting)
			client.Privmsg(event.Arguments[0], greeting)
		}
	});

	// PRIVMSG handles both private and channel messages
	client.AddCallback("PRIVMSG", func(event *irc.Event) {
		if (strings.HasPrefix(event.Arguments[0], "#")) {
			// channel message
			log.Debug("[nick %v] Received PRIVMSG in channel %v from %v: %v", client.GetNick(), event.Arguments[0], event.Nick, event.Message())

			// reply on highlight
			if (strings.Contains(event.Message(), client.GetNick())) {
				client.Privmsg(event.Arguments[0], greeting)
			}
		} else {
			// private message
			log.Debug("[nick %v] Received PRIVMSG from %v: %v", client.GetNick(), event.Nick, event.Message())

			// reply on highlight
			if (strings.Contains(event.Message(), client.GetNick())) {
				client.Privmsg(event.Nick, greeting)
			}
		}
	});

	// connect
	log.Debug("[nick %v] connecting to irc host %v from %v", client.GetNick(), host, original)
	client.Connect(host + ":" + port)
	client.Loop()
}

func ircServer(protocol string) {
	port := *f_ircport
	settings := goircd.Settings{"localhost", ":" + port, "", "", "", "", "", "", false}
	goircd.SetSettings(settings)

	events := make(chan goircd.ClientEvent)

	// Dummy logger (iterated for goircd)
	go func() {
		for _ = range goircd.LogSink {
		}
	}()

	// Dummy statekeeper (iterated for goircd)
	go func() {
		for _ = range goircd.StateSink {
		}
	}()

	// setup listener
	listener, err := net.Listen("tcp", settings.Bind)
	if err != nil {
		log.Fatal("Cannot listen on %s: %v", settings.Bind, err)
	}
	log.Debug("Server listening on %v", settings.Bind)

	go func(sock net.Listener, events chan goircd.ClientEvent) {
		for {
			conn, err := sock.Accept()
			if err != nil {
				log.Fatal("Error during accepting connection %v", err)
				continue
			}
			client := goircd.NewClient(conn)
			go client.Processor(events)
		}
	}(listener, events)

	goircd.Processor(events, make(chan struct{}))
}

func randomNick() string {
	return nicks[rand.Intn(len(nicks))]
}

func randomChannel() string {
	return channels[rand.Intn(len(channels))]
}

func randomMessage(channels []string) (channel string, message string) {
	if len(channels) < 1 {
		return "",""
	}
	channel = channels[rand.Intn(len(channels))]
	message = messages[rand.Intn(len(messages))]
	return
}
