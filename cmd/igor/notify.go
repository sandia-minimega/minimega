// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"os/exec"
	"text/template"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var cmdNotify = &Command{
	UsageLine: "notify",
	Short:     "notify users of upcoming and expiring reservations",
	Long: `
Notify users of upcoming and expiring reservations. Intended to be run as a
cronjob rather than by the users themselves.`,
	Run: runNotify,
}

func runNotify(cmd *Command, args []string) {
	log.Info("notifying users of upcoming and expiring reservations")

	if igor.Username != "root" {
		log.Fatalln("only root can notify users")
	}

	if igor.Domain == "" {
		log.Fatalln("must specify domain in config to notify users")
	}

	type Reservation struct {
		Name  string
		Start time.Time
		End   time.Time
	}
	type Notification struct {
		Upcoming []Reservation
		Expiring []Reservation
	}

	// per-user notification
	users := map[string]*Notification{}

	// TODO: probably shouldn't iteration over .M directly
	for _, r := range igor.Reservations.M {
		// convert unix to time.Time
		res := Reservation{
			Name:  r.Name,
			Start: r.Start,
			End:   r.End,
		}

		// upcoming reservations start in just over an hour
		diff := res.Start.Sub(igor.Now)
		if diff >= 0 && diff < time.Hour {
			if users[r.Owner] == nil {
				users[r.Owner] = &Notification{}
			}
			users[r.Owner].Upcoming = append(users[r.Owner].Upcoming, res)
		}

		// expiring reservations are longer than two days and expire in just
		// over 24 hours.
		diff = res.End.Sub(igor.Now)
		var lowerwindow, upperwindow time.Duration

		if igor.ExpirationLeadTime < 24*60 { //check if there is a leadtime configured if not assign default value
			lowerwindow = time.Duration(23)
			upperwindow = time.Duration(24)
		} else {
			lowerwindow = time.Duration((igor.ExpirationLeadTime / 60) - 1)
			upperwindow = time.Duration(igor.ExpirationLeadTime / 60)
		}
		if res.End.Sub(res.Start) >= 48*time.Hour && diff >= lowerwindow*time.Hour && diff < upperwindow*time.Hour {
			if users[r.Owner] == nil {
				users[r.Owner] = &Notification{}
			}
			users[r.Owner].Expiring = append(users[r.Owner].Expiring, res)
		}
	}

	t := template.Must(template.New("notify").Parse(notifyTemplate))

	for user, notification := range users {
		var buf bytes.Buffer
		if err := t.Execute(&buf, notification); err != nil {
			log.Fatal("template error: %v", err)
		}

		log.Info("notifying %v of %v upcoming and %v expiring reservations", user, len(notification.Upcoming), len(notification.Expiring))

		cmd := exec.Command("mail", "-s", notifySubject, user+"@"+igor.Domain)
		cmd.Stdin = &buf
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Error("mail failed: %v %v", out, err)
		}
	}
}

const notifySubject = "igor reservations"

const notifyTemplate = `
Hello,
{{ if .Upcoming }}
Upcoming reservations:
{{ range $v := .Upcoming }}
	{{- $start := ($v.Start.Format "Mon Jan 2 15:04") }}
	{{- $end := ($v.End.Format "Mon Jan 2 15:04") }}
	{{ printf "%v to %v: %v" $start $end $v.Name }}
{{ end }}
{{ end }}
{{ if .Expiring }}
Expiring reservations:
{{ range $v := .Expiring }}
	{{- $start := ($v.Start.Format "Mon Jan 2 15:04") }}
	{{- $end := ($v.End.Format "Mon Jan 2 15:04") }}
	{{ printf "%v to %v: %v" $start $end $v.Name }}
{{ end }}
{{ end }}
Sincerly,
igor`
