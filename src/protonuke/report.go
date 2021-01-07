// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"bytes"
	"fmt"
	log "minilog"
	"text/tabwriter"
	"time"
)

var (
	dnsReportChan     chan uint64
	ftpReportChan     chan uint64
	ftpTLSReportChan  chan uint64
	httpReportChan    chan uint64
	httpTLSReportChan chan uint64
	sshReportChan     chan uint64
	smtpReportChan    chan uint64

	dnsReportHits     uint64
	ftpReportHits     uint64
	ftpTLSReportHits  uint64
	httpReportHits    uint64
	httpTLSReportHits uint64
	sshReportBytes    uint64
	smtpReportMail    uint64
)

func init() {
	dnsReportChan = make(chan uint64, 1024)
	ftpReportChan = make(chan uint64, 1024)
	ftpTLSReportChan = make(chan uint64, 1024)
	httpReportChan = make(chan uint64, 1024)
	httpTLSReportChan = make(chan uint64, 1024)
	sshReportChan = make(chan uint64, 1024)
	smtpReportChan = make(chan uint64, 1024)

	go func() {
		for {
			select {
			case <-dnsReportChan:
				dnsReportHits++
			case <-ftpReportChan:
				ftpReportHits++
			case <-ftpTLSReportChan:
				ftpTLSReportHits++
			case <-httpReportChan:
				httpReportHits++
			case <-httpTLSReportChan:
				httpTLSReportHits++
			case i := <-sshReportChan:
				sshReportBytes += i
			case <-smtpReportChan:
				smtpReportMail++
			}
		}
	}()
}

func report(reportWait time.Duration) {
	lastTime := time.Now()

	lastDnsReportHits := dnsReportHits
	lastftpReportHits := ftpReportHits
	lastftpTLSReportHits := ftpTLSReportHits
	lasthttpReportHits := httpReportHits
	lasthttpTLSReportHits := httpTLSReportHits
	lastsshReportBytes := sshReportBytes
	lastsmtpReportMail := smtpReportMail

	for {
		time.Sleep(reportWait)
		elapsedTime := time.Since(lastTime)
		lastTime = time.Now()

		edns := dnsReportHits - lastDnsReportHits
		eftp := ftpReportHits - lastftpReportHits
		eftptls := ftpTLSReportHits - lastftpTLSReportHits
		ehttp := httpReportHits - lasthttpReportHits
		etls := httpTLSReportHits - lasthttpTLSReportHits
		essh := sshReportBytes - lastsshReportBytes
		esmtp := smtpReportMail - lastsmtpReportMail

		lastDnsReportHits = dnsReportHits
		lastftpReportHits = ftpReportHits
		lastftpTLSReportHits = ftpTLSReportHits
		lasthttpReportHits = httpReportHits
		lasthttpTLSReportHits = httpTLSReportHits
		lastsshReportBytes = sshReportBytes
		lastsmtpReportMail = smtpReportMail

		log.Debugln("total elapsed time: ", elapsedTime)

		buf := new(bytes.Buffer)
		w := new(tabwriter.Writer)
		w.Init(buf, 0, 8, 0, '\t', 0)

		if *f_dns {
			fmt.Fprintf(w, "dns\t%v\t%.01f hits/min\n", dnsReportHits, float64(edns)/elapsedTime.Minutes())
		}
		if *f_ftp {
			fmt.Fprintf(w, "ftp\t%v\t%.01f hits/min\n", ftpReportHits, float64(eftp)/elapsedTime.Minutes())
		}
		if *f_ftps {
			fmt.Fprintf(w, "ftps\t%v\t%.01f hits/min\n", ftpTLSReportHits, float64(eftptls)/elapsedTime.Minutes())
		}
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
