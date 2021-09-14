// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	log "minilog"
	"version"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

var (
	f_port    = flag.Int("port", 9002, "port to connect to")
	f_version = flag.Bool("version", false, "print the version")
	f_parent  = flag.String("parent", "", "parent to connect to (if relay or client)")
	f_path    = flag.String("path", "/tmp/miniccc", "path to store files in")
	f_serial  = flag.String("serial", "", "use serial device instead of tcp")
	f_family  = flag.String("family", "tcp", "[tcp,unix] family to dial on")
	f_pipe    = flag.String("pipe", "", "read/write to or from a named pipe")
	f_install = flag.String("install", "", "install as Windows service ('manual-start' or 'auto-start')")
)

const banner = `miniccc.exe, Copyright (2014) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.
`

func usage() {
	fmt.Println(banner)
	fmt.Println("usage: miniccc.exe [option]... ")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if *f_version {
		fmt.Println("miniccc.exe", version.Revision, version.Date)
		fmt.Println(version.Copyright)
		os.Exit(0)
	}

	log.Init()

	if *f_install != "" {
		// The `-install` flag was passed, so create a Windows Service for miniccc
		// and exit.
		if err := installService(); err != nil {
			log.Fatal("installing Windows service: %v", err)
		}

		return
	}

	// init client
	NewClient()

	if *f_pipe != "" {
		pipeHandler(*f_pipe)
		return
	}

	// attempt to set up the base path
	if err := os.MkdirAll(*f_path, os.FileMode(0777)); err != nil {
		log.Fatal("mkdir base path: %v", err)
	}

	log.Info("starting ron client with UUID: %v", client.UUID)

	inInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		log.Fatal("detecting interactive session: %v", err)
	}

	// Not running interactively means this code was called by the Windows Service
	// Manager Controller, so run as a Windows Service (and all the dumb magic
	// this entails/requires).
	if !inInteractive {
		var err error

		elog, err = eventlog.Open(svcName)
		if err != nil {
			log.Fatal("opening Windows event log: %v", err)
		}

		defer elog.Close()

		elog.Info(1, fmt.Sprintf("starting %s service", svcName))

		// this blocks
		if err := svc.Run(svcName, &miniSvc{}); err != nil {
			elog.Error(1, fmt.Sprintf("%s service failed: %v", svcName, err))
			log.Fatal("running Windows service: %v", err)
		}

		elog.Info(1, fmt.Sprintf("%s service stopped", svcName))

		return
	}

	// Running interactively, which means business as usual.
	resetClient()

	// wait for SIGTERM
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

var done chan struct{}

// called by mux if loss of server is detected
func resetClient() {
	// On Windows, trying to redial the virtual serial ports always results in an
	// `Access denied` error from Windows. Instead, just exit and let Windows
	// restart the failed service. If running interactively, then the user will
	// have to restart manually.
	if done != nil {
		close(done)
		log.Fatal("reset called after initialization - aborting")
	}

	if err := dial(); err != nil {
		log.Fatal("unable to connect: %v", err)
	}

	done = make(chan struct{})

	go mux(done)
	heartbeat() // handshake is first heartbeat
}

const (
	svcName = "miniccc"
	svcDesc = "minimega Agent"
)

var elog *eventlog.Log

type miniSvc struct{}

func (this *miniSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	// This is all the dumb magic required to run as a service on Windows. Luckily
	// Go makes it pretty easy to do.
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	// kickoff miniccc
	resetClient()

	// manage Windows service manager controller requests and changes
	for {
		c := <-r

		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			elog.Info(1, "miniccc shutting down")
			changes <- svc.Status{State: svc.StopPending}

			return
		default:
			elog.Error(1, fmt.Sprintf("unexpected control request #%d", c))
		}
	}
}

func installService() error {
	// figure out what directory miniccc.exe was called from
	exePath := func() (string, error) {
		prog := os.Args[0]

		p, err := filepath.Abs(prog)
		if err != nil {
			return "", err
		}

		fi, err := os.Stat(p)
		if err == nil {
			if !fi.Mode().IsDir() {
				return p, nil
			}

			err = fmt.Errorf("%s is directory", p)
		}

		if filepath.Ext(p) == "" {
			p += ".exe"

			fi, err := os.Stat(p)
			if err == nil {
				if !fi.Mode().IsDir() {
					return p, nil
				}

				err = fmt.Errorf("%s is directory", p)
			}
		}

		return "", err
	}

	exepath, err := exePath()
	if err != nil {
		return err
	}

	// connect to the Windows Service Manager Controller
	m, err := mgr.Connect()
	if err != nil {
		return err
	}

	defer m.Disconnect()

	s, err := m.OpenService(svcName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", svcName)
	}

	c := mgr.Config{DisplayName: svcDesc}

	switch *f_install {
	case "manual-start":
		c.StartType = mgr.StartManual
	case "auto-start":
		c.StartType = mgr.StartAutomatic
	default:
		c.StartType = mgr.StartManual
	}

	// When using the `-install` flag, use the `-logfile` and `-level` flags to
	// set logging options for the service being installed.
	s, err = m.CreateService(svcName, exepath, c, "-serial", "\\\\.\\Global\\cc", "-logfile", log.FileFlag, "-level", log.LevelFlag.String())
	if err != nil {
		return err
	}

	defer s.Close()

	r := []mgr.RecoveryAction{
		{
			Type:  mgr.ServiceRestart,
			Delay: 5 * time.Second,
		},
	}

	// set the Windows service to restart miniccc.exe if it fails
	if err := s.SetRecoveryActions(r, 0); err != nil {
		s.Delete()
		return err
	}

	// create Windows event log for the miniccc.exe service
	eventlog.InstallAsEventCreate(svcName, eventlog.Error|eventlog.Warning|eventlog.Info)

	return nil
}
