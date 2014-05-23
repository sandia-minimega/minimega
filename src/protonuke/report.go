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
	smtpReportChan    chan uint64

	httpTimeBuffer    chan uint64
	httpTLSTimeBuffer chan uint64

	httpReportHits    uint64
	httpTLSReportHits uint64
	sshReportBytes    uint64
	smtpReportMail    uint64
)

func init() {
	httpReportChan = make(chan uint64, 1024)
	httpTLSReportChan = make(chan uint64, 1024)
	sshReportChan = make(chan uint64, 1024)
	smtpReportChan = make(chan uint64, 1024)

	httpTimeBuffer = make(chan uint64, 1000000)
	httpTLSTimeBuffer = make(chan uint64, 1000000)

	go func() {
		for {
			select {
			case i := <-httpReportChan:
				httpTimeBuffer <- i
				httpReportHits++
			case i := <-httpTLSReportChan:
				httpTLSTimeBuffer <- i
				httpTLSReportHits++
			case i := <-sshReportChan:
				sshReportBytes += i
			case i := <-smtpReportChan:
				smtpReportMail += i
			}
		}
	}()
}

func report(reportWait time.Duration) {
	lastTime := time.Now()

	lasthttpReportHits := httpReportHits
	lasthttpTLSReportHits := httpTLSReportHits
	lastsshReportBytes := sshReportBytes
	lastsmtpReportMail := smtpReportMail

	for {
		time.Sleep(reportWait)
		elapsedTime := time.Since(lastTime)
		lastTime = time.Now()

		ehttp := httpReportHits - lasthttpReportHits
		etls := httpTLSReportHits - lasthttpTLSReportHits
		essh := sshReportBytes - lastsshReportBytes
		esmtp := smtpReportMail - lastsmtpReportMail
		lasthttpReportHits = httpReportHits
		lasthttpTLSReportHits = httpTLSReportHits
		lastsshReportBytes = sshReportBytes
		lastsmtpReportMail = smtpReportMail

		log.Debugln("total elapsed time: ", elapsedTime)

		fmt.Printf("HTTP timing:\t")
		for i := uint64(0); i < ehttp; i++ {
			fmt.Printf("%d ", <-httpTimeBuffer)
		}
		fmt.Printf("\n")

		fmt.Printf("HTTPS timing:\t")
		for i := uint64(0); i < etls; i++ {
			fmt.Printf("%d ", <-httpTLSTimeBuffer)
		}
		fmt.Printf("\n")

		buf := new(bytes.Buffer)
		w := new(tabwriter.Writer)
		w.Init(buf, 0, 8, 0, '\t', 0)

		if *f_http {
			fmt.Fprintf(w, "http\t%v\t%.01f hits/min\n", httpReportHits, float64(ehttp)/elapsedTime.Minutes())
		}
		if *f_https {
			fmt.Fprintf(w, "https\t%v\t%.01f hits/min\n", httpTLSReportHits, float64(etls)/elapsedTime.Minutes())
		}
		if *f_ssh {
			fmt.Fprintf(w, "ssh\t%v\t%.01f bytes/min\n", sshReportBytes, float64(essh)/elapsedTime.Minutes())
		}
		if *f_smtp {
			fmt.Fprintf(w, "smtp\t%v\t%.01f mails/min\n", smtpReportMail, float64(esmtp)/elapsedTime.Minutes())
		}
		w.Flush()
		fmt.Println(buf.String())
	}
}
