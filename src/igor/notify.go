// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	log "minilog"
	"os/exec"
	"text/template"
	"time"
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

	user, err := getUser()
	if err != nil {
		log.Fatalln("Cannot determine current user", err)
	}

	if user.Username != "root" {
		log.Fatalln("only root can notify users")
	}

	if igorConfig.Domain == "" {
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

	now := time.Now()

	for _, r := range Reservations {
		// convert unix to time.Time
		res := Reservation{
			Name:  r.ResName,
			Start: time.Unix(r.StartTime, 0),
			End:   time.Unix(r.EndTime, 0),
		}

		// upcoming reservations start in just over an hour
		diff := res.Start.Sub(now)
		if diff >= 0 && diff < time.Hour {
			if users[r.Owner] == nil {
				users[r.Owner] = &Notification{}
			}
			users[r.Owner].Upcoming = append(users[r.Owner].Upcoming, res)
		}

		// expiring reservations are longer than two days and expire in just
		// over 24 hours.
		diff = res.End.Sub(now)
		var lowerwindow, upperwindow time.Duration

		if igorConfig.ExpirationLeadTime < 24 { //check if there is a leadtime configured if not assign default value
			lowerwindow = time.Duration(23)
			upperwindow = time.Duration(24)
		} else {
			lowerwindow = time.Duration((igorConfig.ExpirationLeadTime / 60) - 1)
			upperwindow = time.Duration(igorConfig.ExpirationLeadTime / 60)
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

		cmd := exec.Command("mail", "-s", notifySubject, user+"@"+igorConfig.Domain)
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
