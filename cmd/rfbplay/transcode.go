// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os/exec"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// transcode runs ffmpeg to transcode the MJPEG stream at the URL src to the
// file out.
func transcode(src, out string) error {
	p := "ffmpeg"

	var args []string
	args = append(args, "-f")
	args = append(args, "mjpeg")
	args = append(args, "-r")
	args = append(args, "10") // minimega uses 10 frames per second
	args = append(args, "-i")
	args = append(args, src)
	args = append(args, out)

	log.Debugln("args:", args)

	cmd := exec.Command(p, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	log.LogAll(stdout, log.INFO, "ffmpeg")
	log.LogAll(stderr, log.INFO, "ffmpeg")

	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
