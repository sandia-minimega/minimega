// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"time"
	"io/ioutil"
	"errors"
	"strings"
	"os/exec"
	"path/filepath"
	"os"
)

const (
	injectCommand = iota
	getBackingImageCommand
)

type injectPair struct {
	src string
	dst string
}

type injectData struct {
	childImg string
	srcImg string
	dstImg string
	partition string
	nPairs int
	injPairs []injectPair
}

//Parse the source-file:destination pairs
func (inject *injectData) parseInjectPairs(c cliCommand, argIdx int) error {
	var args string
	parseSrc := true
	injIdx := 0

	//parse inject pairs
	args = strings.Join(c.Args[argIdx:], " ")
	quotedTokens := strings.Split(args, "\"")
	for _, quotedTok := range quotedTokens {
		if quotedTok == "" || quotedTok == " " {
			continue
		}
		
		//if there is no ":", path is quoted
		if !strings.Contains(quotedTok, ":") {
			if parseSrc {
				inject.injPairs[injIdx].src = quotedTok
				parseSrc = false
			} else {
				inject.injPairs[injIdx].dst = quotedTok
				parseSrc = true
				injIdx++
			}
			continue
		} else {
			//nothing in this token was quoted,
			//so both spaces and : split arguments
			st := strings.Replace(quotedTok, ":", " ", -1)
			toks := strings.Split(st, " ")
			for _, tok := range toks {
				if tok == "" || tok == " " {
					continue
				}
				
				if parseSrc {
					inject.injPairs[injIdx].src = tok
					parseSrc = false
				} else {
					inject.injPairs[injIdx].dst = tok
					parseSrc = true
					injIdx++
				}
			}
		}
	}
	
	inject.nPairs = injIdx
	
	if !parseSrc {
		return errors.New("malformed command")
	}

	return nil
}

//Parse the command line to get the arguments for vm_inject
func (inject *injectData) parseInject(c cliCommand) error {
	argIdx := 1
	var dstImgStr string
	var dstImg *os.File
	var err error

	switch {
	case len(c.Args) == 0:
		return errors.New("malformed command")
	case len(c.Args) == 1:
		inject.childImg = c.Args[0]
		return nil
	case len(c.Args) > 1:
		inject.injPairs = make([]injectPair, len(c.Args) - 1)
		
		//parse source image
		srcPair := strings.Split(c.Args[0], ":")
		inject.srcImg, err = filepath.Abs(srcPair[0])
		if err != nil {
			return err
		}
		if len(srcPair) == 2 {
			inject.partition = srcPair[1]
		}
		
		//parse destination image
		if !strings.Contains(c.Args[1], ":") {
			if strings.Contains(c.Args[1], "/") {
				return errors.New("dst image path must not be absolute")
			}
			dstImgStr = *f_base + c.Args[1]
			argIdx++;
		} else {
			dstImg, err = ioutil.TempFile(*f_base, "snapshot")
			dstImgStr = dstImg.Name()
			if err != nil {
				return errors.New("could not create a dst image")
			}
		}
		inject.dstImg = dstImgStr

		return inject.parseInjectPairs(c, argIdx)

	}
	return nil
}

//Unmount, disconnect nbd, and remove mount directory
func vmInjectCleanup(mntDir string) {
	cmd := exec.Command("umount", mntDir)
	_ = cmd.Run()
	
	cmd = exec.Command("qemu-nbd", "-d", "/dev/nbd0")
	_ = cmd.Run()

	cmd = exec.Command("rm", "-r", mntDir)
	_ = cmd.Run()
}

//Inject files into a qcow
//Alternatively, this function can also return a qcow2 backing file's name
func cliVMInject(c cliCommand) cliResponse {
	r := cliResponse{}
	inject := injectData{}

	err := inject.parseInject(c)
	if err != nil {
		r.Error = err.Error()
		return r
	}

	if inject.childImg != "" {
		cmd := exec.Command("qemu-img", "info", inject.childImg)
		parent, err := cmd.Output()
		if err != nil {
			r.Error = err.Error()
		} else {
			r.Response = string(parent[:])
		}
		return r
	}

	//create the new img
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", "-b",
		inject.srcImg, inject.dstImg)
	result, err := cmd.CombinedOutput()
	if err != nil {
		r.Error = string(result[:]) + "\n" + err.Error()
		return r
	}
	
	//create a tmp mount point
	mntDir, err := ioutil.TempDir(*f_base, "dstImg")
	if err != nil {
		r.Error = err.Error()
		return r
	}
	
	//connect new img to nbd
	cmd = exec.Command("qemu-nbd", "-c", "/dev/nbd0", 
		inject.dstImg)
	result, err = cmd.CombinedOutput()
	if err != nil {
		vmInjectCleanup(mntDir)
		r.Error = string(result[:]) + "\n" + err.Error()
		return r
	}
	
	time.Sleep(100 * time.Millisecond) //give time to create partitions
	
	//decide on a partition
	if inject.partition == "" {
		_, err = os.Stat("/dev/nbd0p1")
		if err != nil {
			vmInjectCleanup(mntDir)
			r.Error = "no partitions found"
			return r
		}
		
		_, err = os.Stat("/dev/nbd0p2")

		if err == nil {
			vmInjectCleanup(mntDir)
			r.Error = "please specify a partition; multiple found"
			return r
		}

		inject.partition = "1"
	}
	
	//mount new img
	cmd = exec.Command("mount", "-w", "/dev/nbd0p" + inject.partition, 
		mntDir)
	result, err = cmd.CombinedOutput()
	if err != nil {
		//if mount failed, try ntfs-3g
		cmd = exec.Command("mount", "-o", "ntfs-3g", 
			"/dev/nbd0p" + inject.partition, mntDir)
		result, err = cmd.CombinedOutput()
		if err != nil {
			vmInjectCleanup(mntDir)
			r.Error = string(result[:]) + "\n" + err.Error()
			return r
		}
	}
	
	//copy files/folders in
	for i := 0; i < inject.nPairs; i++ {
		cmd = exec.Command("cp", "-r", inject.injPairs[i].src, 
			mntDir + "/" + inject.injPairs[i].dst)
		result, err = cmd.CombinedOutput()
		if err != nil {
			vmInjectCleanup(mntDir)
			r.Error = string(result[:]) + "\n" + err.Error()
			return r
			}
	}

	vmInjectCleanup(mntDir)
	return r
}