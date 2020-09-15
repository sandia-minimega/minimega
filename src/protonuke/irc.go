// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"io/ioutil"
	"math/rand"
	log "minilog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stargrave/goircd"
	"github.com/thoj/go-ircevent"
)

const (
	greeting         = "yo"
	pairQuitTime     = 60
	convoWaitTimeMin = 5
	convoWaitTimeMax = 30
)

var (
	channels []string

	messages = defaultMessages
	nicks    = defaultNicks

	usersLock   sync.Mutex
	channelLock sync.Mutex
)

type Conversation struct {
	nick      string
	channel   string
	isPaired  bool
	isWaiting bool
	counter   int
}

func updateIRCHitCount() {
	ircReportChan <- 1
}

func ircClient() {
	t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)
	log.Debugln("ircClient")
	rand.Seed(time.Now().UnixNano())

	// handle passed flags
	port := *f_ircport
	channels = strings.Split(*f_channels, ",")
	if *f_messages != "" {
		data, err := ioutil.ReadFile(*f_messages)
		if err != nil {
			log.Fatal("Unable to read file %v", *f_messages)
		}
		messages = strings.Split(strings.Replace(string(data), "\r", "", -1), "\n")
	}
	chain := NewChain()
	if *f_markov {
		chain.Build(messages)
	}

	host, original := randomHost()
	nick := randomNick()
	client := irc.IRC(nick, nick)
	client.HitCallback(updateIRCHitCount)

	// generate list of channels to join
	n := 1
	userChannels := []string{}
	if len(channels) > 1 {
		n += rand.Intn(len(channels) - 1)
	}
	if len(channels) > 0 {
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
	}
	joinedChannels := []string{}
	channelUsers := make(map[string]string)
	pair := Conversation{"", "", false, false, 0}

	// create callbacks
	// 001: RPL_WELCOME "Welcome to the Internet Relay Network <nick>!<user>@<host>"
	client.AddCallback("001", func(event *irc.Event) {
		// do irc communication on a separate thread (not main thread)
		go func(event *irc.Event) {
			// join channels
			for i := 0; i < len(userChannels); i++ {
				t.Tick()
				log.Debug("[nick %v] joining channel %v on host %v", client.GetNick(), userChannels[i], host)
				client.Join(userChannels[i])
			}

			// have some nice conversations
			if *f_markov {
				// use markov chain
				for {
					t.Tick()
					if len(joinedChannels) == 0 {
						// wait until nick is in a channel
						continue
					}

					if pair.isWaiting {
						pair.counter++
						if pair.counter > pairQuitTime {
							// nick did not reply, give up and reset conversation flags
							pair.nick = ""
							pair.channel = ""
							pair.isWaiting = false
							pair.isPaired = false
							pair.counter = 0
						}
					} else if !pair.isPaired {
						// idle for a bit before starting a new conversation
						wait := rand.Intn(convoWaitTimeMax-convoWaitTimeMin) + convoWaitTimeMin
						for i := 0; i < wait; i++ {
							t.Tick()
						}

						// get random channel and user
						channel := randomFromSlice(joinedChannels)

						// check if channel is empty
						if len(channelUsers[channel]) > 0 {
							nick := randomFromSlice(strings.Split(channelUsers[channel], " "))

							// set conversation flags
							pair.isWaiting = true
							pair.nick = nick
							pair.channel = channel

							// ping user
							log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), channel, nick)
							client.Privmsg(channel, nick)
						}

					}
				}
			} else {
				// random
				for {
					t.Tick()
					if len(joinedChannels) == 0 {
						// wait until nick is in a channel
						continue
					}

					to := randomFromSlice(joinedChannels)
					message := randomMessage()
					log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), to, message)
					client.Privmsg(to, message)
				}
			}
		}(event)
	})

	// 353: RPL_NAMREPLY "= <channel> :<names>"
	client.AddCallback("353", func(event *irc.Event) {
		// add users to channel mapping
		go func(event *irc.Event) {
			usersLock.Lock()
			defer usersLock.Unlock()

			nicks := strings.Split(event.Message(), " ")
			channel := event.Arguments[2]

			nick = client.GetNick()
			for i, n := range nicks {
				if n == nick {
					nicks = append(nicks[:i], nicks[i+1:]...)
					break
				}
			}
			channelUsers[channel] = strings.Join(nicks, " ")
		}(event)
	})

	// 433: ERR_NICKNAMEINUSE "<nick> :Nickname is already in use"
	client.AddCallback("433", func(event *irc.Event) {
		// Note:  removed 433 callback from go-ircevent where go-ircevent will receive
		//        the callback first and append underscores to the name instead

		// append random number to nick
		newNick := nick + strconv.Itoa(rand.Intn(1000000))
		log.Debug("[nick %v] Switching nick to %v", client.GetNick(), newNick)
		client.Nick(newNick)
	})

	// JOIN occurs after you successfully join a channel
	client.AddCallback("JOIN", func(event *irc.Event) {
		if event.Nick == client.GetNick() {
			// if self, add channel to list of joined channels
			channelLock.Lock()
			defer channelLock.Unlock()

			// add channel to joined channel slice
			joinedChannels = append(joinedChannels, event.Arguments[0])

			// send greeting to channel
			log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), event.Arguments[0], greeting)
			client.Privmsg(event.Arguments[0], greeting)
		} else {
			// else add nick to users in channel
			usersLock.Lock()
			defer usersLock.Unlock()

			nick := event.Nick
			channel := event.Arguments[0]

			nicks := strings.Split(channelUsers[channel], " ")
			nicks = append(nicks, nick)
			channelUsers[channel] = strings.Join(nicks, " ")
		}
	})

	// PRIVMSG handles both private and channel messages
	client.AddCallback("PRIVMSG", func(event *irc.Event) {
		if strings.HasPrefix(event.Arguments[0], "#") {
			// channel message
			channel := event.Arguments[0]
			message := event.Message()
			log.Debug("[nick %v] Received PRIVMSG in channel %v from %v: %v", client.GetNick(), channel, event.Nick, message)

			if pair.isWaiting && (pair.nick == event.Nick) {
				if !pair.isPaired {
					// either paired nick confirmed the conversation with us or ignored it
					if strings.Contains(message, greeting) {
						pair.isPaired = true
						pair.isWaiting = false
						pair.counter = 0
						t.Tick()

						if *f_markov {
							message = chain.Generate()
						} else {
							message = randomMessage()
						}

						log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), channel, message)
						client.Privmsg(channel, message)
						pair.isWaiting = true
					}
				} else {
					// already paired with nick, so respond
					pair.isPaired = true
					pair.isWaiting = false
					pair.counter = 0
					t.Tick()

					if *f_markov {
						message = chain.Generate()
					} else {
						message = randomMessage()
					}

					log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), channel, message)
					client.Privmsg(channel, message)
					pair.isWaiting = true
				}
			} else if strings.HasPrefix(message, client.GetNick()) && (!pair.isWaiting && !pair.isPaired) {
				// another nick has requested a conversation, accept
				pair.isPaired = true
				pair.isWaiting = false
				pair.counter = 0
				t.Tick()

				log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), channel, greeting)
				client.Privmsg(channel, greeting)
				pair.isWaiting = true
			}
		} else {
			// private message, don't consider as a pair/conversation
			log.Debug("[nick %v] Received PRIVMSG from %v: %v", client.GetNick(), event.Nick, event.Message())

			if strings.Contains(event.Message(), client.GetNick()) {
				// reply to highlight with greeting
				log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), event.Nick, greeting)
				client.Privmsg(event.Nick, greeting)
			} else {
				// otherwise, create a new message
				if *f_markov {
					message := chain.Generate()
					log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), event.Nick, message)
					client.Privmsg(event.Nick, message)
				} else {
					message := randomMessage()
					log.Debug("[nick %v] Sending PRIVMSG to %v: %v", client.GetNick(), event.Nick, message)
					client.Privmsg(event.Nick, message)
				}
			}
		}
	})

	// PART occurs when someone leaves the channel
	client.AddCallback("PART", func(event *irc.Event) {
		// remove the user from channel
		go func(event *irc.Event) {
			usersLock.Lock()
			defer usersLock.Unlock()

			nick := event.Nick
			channel := event.Arguments[0]
			nicks := strings.Split(channelUsers[channel], " ")
			for i, n := range nicks {
				if n == nick {
					nicks := append(nicks[:i], nicks[i+1:]...)
					channelUsers[channel] = strings.Join(nicks, " ")
					break
				}
			}
		}(event)
	})

	// QUIT occurs when someone leaves the server
	client.AddCallback("QUIT", func(event *irc.Event) {
		// remove the user from all channels
		go func(event *irc.Event) {
			usersLock.Lock()
			defer usersLock.Unlock()

			nick := event.Nick
			for channel, v := range channelUsers {
				nicks := strings.Split(v, " ")
				for i, n := range nicks {
					if n == nick {
						nicks = append(nicks[:i], nicks[i+1:]...)
						channelUsers[channel] = strings.Join(nicks, " ")
						break
					}
				}
			}
		}(event)
	})

	// connect
	log.Debug("[nick %v] connecting to irc host %v from %v", client.GetNick(), host, original)
	for {
		err := client.Connect(host + ":" + port)
		if err == nil {
			break
		} else {
			log.Error("%v", err)
		}
	}
	client.Loop()
}

func ircServer() {
	port := *f_ircport
	settings := goircd.Settings{Hostname: "localhost", Bind: ":" + port, Motd: "", Logdir: "", Statedir: "", Passwords: "", TlsBind: "", TlsPEM: "", Verbose: false}
	goircd.HitCallback(updateIRCHitCount)
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

func randomFromSlice(slice []string) string {
	if len(slice) < 1 {
		return ""
	}
	return slice[rand.Intn(len(slice))]
}

func randomNick() string {
	return nicks[rand.Intn(len(nicks))]
}

func randomChannel() string {
	if len(channels) < 1 {
		return ""
	}
	return channels[rand.Intn(len(channels))]
}

func randomMessage() string {
	if len(messages) < 1 {
		return ""
	}
	return messages[rand.Intn(len(messages))]
}
