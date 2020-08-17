package web

import (
	"context"
	"encoding/json"
	"flag"
	"regexp"

	"gophenix/web/broker"

	"github.com/hpcloud/tail"
)

var (
	f_mmLogFile       string
	f_phenixLogFile   string
	f_gophenixLogFile string
	f_serviceLogs     bool

	logLineRegex = regexp.MustCompile(`\A(\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2})\s* (DEBUG|INFO|WARN|WARNING|ERROR|FATAL) .*?: (.*)\z`)
)

func init() {
	flag.BoolVar(&f_serviceLogs, "log.publish", false, "publish service logs to UI (gophenix, phenix, minimega logs)")
	flag.StringVar(&f_mmLogFile, "log.mm-file", "", "path to minimega log file")
	flag.StringVar(&f_phenixLogFile, "log.phenix-file", "", "path to phenix log file")
	flag.StringVar(&f_gophenixLogFile, "log.gophenix-file", "", "path to gophenix log file")
}

func PublishLogs(ctx context.Context) {
	if !f_serviceLogs {
		return
	}

	mmLogs, err := tail.TailFile(f_mmLogFile, tail.Config{Follow: true, ReOpen: true, Poll: true})
	if err != nil {
		panic("setting up tail for minimega logs: " + err.Error())
	}

	phenixLogs, err := tail.TailFile(f_phenixLogFile, tail.Config{Follow: true, ReOpen: true, Poll: true})
	if err != nil {
		panic("setting up tail for phenix logs: " + err.Error())
	}

	gophenixLogs, err := tail.TailFile(f_gophenixLogFile, tail.Config{Follow: true, ReOpen: true, Poll: true})
	if err != nil {
		panic("setting up tail for gophenix logs: " + err.Error())
	}

	// Used to detect multi-line logs in tailed log files.
	var (
		mmBody       map[string]interface{}
		phenixBody   map[string]interface{}
		gophenixBody map[string]interface{}
	)

	for {
		select {
		case <-ctx.Done():
			return
		case log := <-mmLogs.Lines:
			parts := logLineRegex.FindStringSubmatch(log.Text)

			if len(parts) == 4 {
				mmBody = map[string]interface{}{
					"source":    "minimega",
					"timestamp": parts[1],
					"level":     parts[2],
					"log":       parts[3],
				}
			} else if mmBody != nil {
				mmBody["log"] = log.Text
			} else {
				continue
			}

			marshalled, _ := json.Marshal(mmBody)

			broker.Broadcast(
				nil,
				broker.NewResource("log", "minimega", "update"),
				marshalled,
			)
		case log := <-phenixLogs.Lines:
			parts := logLineRegex.FindStringSubmatch(log.Text)

			if len(parts) == 4 {
				if parts[2] == "WARNING" {
					parts[2] = "WARN"
				}

				phenixBody = map[string]interface{}{
					"source":    "phenix",
					"timestamp": parts[1],
					"level":     parts[2],
					"log":       parts[3],
				}
			} else if phenixBody != nil {
				phenixBody["log"] = log.Text
			} else {
				continue
			}

			marshalled, _ := json.Marshal(phenixBody)

			broker.Broadcast(
				nil,
				broker.NewResource("log", "phenix", "update"),
				marshalled,
			)
		case log := <-gophenixLogs.Lines:
			parts := logLineRegex.FindStringSubmatch(log.Text)

			if len(parts) == 4 {
				gophenixBody = map[string]interface{}{
					"source":    "gophenix",
					"timestamp": parts[1],
					"level":     parts[2],
					"log":       parts[3],
				}
			} else if gophenixBody != nil {
				gophenixBody["log"] = log.Text
			} else {
				continue
			}

			marshalled, _ := json.Marshal(gophenixBody)

			broker.Broadcast(
				nil,
				broker.NewResource("log", "gophenix", "update"),
				marshalled,
			)
		}
	}
}
