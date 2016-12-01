// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
    "math/rand"
    log "minilog"
    "strconv"
    "strings"
    "time"

    "github.com/thoj/go-ircevent"
)

const (
    port     = ":6667"
    greeting = "yo"
)

func ircClient(protocol string) {
    t := NewEventTicker(*f_mean, *f_stddev, *f_min, *f_max)

    log.Debugln("ircClient")
    rand.Seed(time.Now().UnixNano())

    host, original := randomHost()

    nick := randomNick()
    client := irc.IRC(nick, nick)

    // generate list of channels to join
    n := rand.Intn(len(channels) - 1) + 1
    userChannels := []string{}
    for i := 0; i < n; i++ {
        channel := randomChannel()
        for _, item := range userChannels {
            if item == channel {
                continue
            }
        }
        userChannels = append(userChannels, channel)
    }


    // create callbacks

    // 001: RPL_WELCOME "Welcome to the Internet Relay Network <nick>!<user>@<host>"
    client.AddCallback("001", func(event *irc.Event) {
        go func(event *irc.Event) {
            // join channels
            for i := 0; i < len(userChannels); i++ {
                log.Debug("[nick %v] joining channel %v on host %v", client.GetNick(), userChannels[i], host)
                client.Join(userChannels[i])
            }

            for {
                t.Tick()
                to, message := randomMessage(userChannels)
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
        client.Privmsg(event.Arguments[0], greeting)
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
    client.Connect(host + port)
    client.Loop()
}

func randomNick() string {
    return nicks[rand.Intn(len(nicks))]
}

func randomChannel() string {
    return channels[rand.Intn(len(channels))]
}

func random() string {
    return channels[rand.Intn(len(channels))]
}

func randomMessage(channels []string) (channel string, message string) {
    channel = channels[rand.Intn(len(channels))]
    message = "mine"
    return
}


var nicks = []string{"theron",
    "thaddeus",
    "seth",
    "perry",
    "brendan",
    "porfirio",
    "jerald",
    "shayne",
    "gino",
    "rickey",
    "elmer",
    "cameron",
    "drew",
    "lucio",
    "francis",
    "christian",
    "jerrell",
    "dirk",
    "jere",
    "kelley",
    "jaimie",
    "holli",
    "larissa",
    "sarah",
    "sophia",
    "terrilyn",
    "stacia",
    "sindy",
    "josphine",
    "janae",
    "violette",
    "gabriella",
    "mellie",
    "asha",
    "vickie",
    "evelynn",
    "clora",
    "linsey",
    "gianna",
    "emelda"}

var channels = []string{
    "#general",
    "#random",
    "#help",
    "#minimega",
    "#development",
    "#irc"}
