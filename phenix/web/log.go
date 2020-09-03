package web

import (
	"context"
	"encoding/json"
	"regexp"

	"phenix/web/broker"

	"github.com/hpcloud/tail"
)

var logLineRegex = regexp.MustCompile(`\A(\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2})\s* (DEBUG|INFO|WARN|WARNING|ERROR|FATAL) .*?: (.*)\z`)

type LogKind int

const (
	_ LogKind = iota
	LOG_PHENIX
	LOG_MINIMEGA
)

type logLine struct {
	kind LogKind
	line string
}

func PublishLogs(ctx context.Context, phenix, minimega string) {
	if phenix == "" && minimega == "" {
		return
	}

	logs := make(chan logLine)

	if phenix != "" {
		phenixLogs, err := tail.TailFile(phenix, tail.Config{Follow: true, ReOpen: true, Poll: true})
		if err != nil {
			panic("setting up tail for phenix logs: " + err.Error())
		}

		go func() {
			for l := range phenixLogs.Lines {
				logs <- logLine{kind: LOG_PHENIX, line: l.Text}
			}
		}()
	}

	if minimega != "" {
		mmLogs, err := tail.TailFile(minimega, tail.Config{Follow: true, ReOpen: true, Poll: true})
		if err != nil {
			panic("setting up tail for minimega logs: " + err.Error())
		}

		go func() {
			for l := range mmLogs.Lines {
				logs <- logLine{kind: LOG_MINIMEGA, line: l.Text}
			}
		}()
	}

	// Used to detect multi-line logs in tailed log files.
	var (
		mmBody     map[string]interface{}
		phenixBody map[string]interface{}
	)

	for {
		select {
		case <-ctx.Done():
			return
		case l := <-logs:
			parts := logLineRegex.FindStringSubmatch(l.line)

			switch l.kind {
			case LOG_PHENIX:
				if len(parts) == 4 {
					phenixBody = map[string]interface{}{
						"source":    "gophenix",
						"timestamp": parts[1],
						"level":     parts[2],
						"log":       parts[3],
					}
				} else if phenixBody != nil {
					phenixBody["log"] = l.line
				} else {
					continue
				}

				marshalled, _ := json.Marshal(phenixBody)

				broker.Broadcast(
					nil,
					broker.NewResource("log", "gophenix", "update"),
					marshalled,
				)
			case LOG_MINIMEGA:
				if len(parts) == 4 {
					mmBody = map[string]interface{}{
						"source":    "minimega",
						"timestamp": parts[1],
						"level":     parts[2],
						"log":       parts[3],
					}
				} else if mmBody != nil {
					mmBody["log"] = l.line
				} else {
					continue
				}

				marshalled, _ := json.Marshal(mmBody)

				broker.Broadcast(
					nil,
					broker.NewResource("log", "minimega", "update"),
					marshalled,
				)
			default:
				continue
			}
		}
	}
}
