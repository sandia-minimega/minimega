package main

import (
	"bytes"
	"fmt"
	log "minilog"
	"text/tabwriter"
	"time"
)

var (
	httpReportChan    chan uint64
	httpTLSReportChan chan uint64
	sshReportChan     chan uint64

	httpReportHits    uint64
	httpTLSReportHits uint64
	sshReportBytes    uint64
)

func init() {
	httpReportChan = make(chan uint64, 1024)
	httpTLSReportChan = make(chan uint64, 1024)
	sshReportChan = make(chan uint64, 1024)

	go func() {
		for {
			select {
			case i := <-httpReportChan:
				httpReportHits += i
			case i := <-httpTLSReportChan:
				httpTLSReportHits += i
			case i := <-sshReportChan:
				sshReportBytes += i
			}
		}
	}()
}

func report(reportWait time.Duration) {
	lastTime := time.Now()
	elapsedTime := time.Duration(0)
	for {
		time.Sleep(reportWait)
		elapsedTime += time.Since(lastTime)
		lastTime = time.Now()

		log.Debugln("total elapsed time: ", elapsedTime)

		buf := new(bytes.Buffer)
		w := new(tabwriter.Writer)
		w.Init(buf, 0, 8, 0, '\t', 0)

		if *f_http {
			fmt.Fprintf(w, "http\t%v\t%.01f hits/min\n", httpReportHits, float64(httpReportHits)/elapsedTime.Minutes())
		}
		if *f_https {
			fmt.Fprintf(w, "https\t%v\t%.01f hits/min\n", httpTLSReportHits, float64(httpTLSReportHits)/elapsedTime.Minutes())
		}
		if *f_ssh {
			fmt.Fprintf(w, "ssh\t%v\t%.01f bytes/min\n", sshReportBytes, float64(sshReportBytes)/elapsedTime.Minutes())
		}
		w.Flush()
		fmt.Println(buf.String())
	}
}
