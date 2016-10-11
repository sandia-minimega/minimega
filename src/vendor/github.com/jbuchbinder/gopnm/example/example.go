// Copyright 2012 Harry de Boer. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/jbuchbinder/gopnm"
	"image"
	"log"
	"os"
)

func main() {
	file, err := os.Open("in.ppm")
	if err != nil {
		log.Fatal(err)
	}

	img, format, err := image.Decode(file)

	log.Println("Format: ", format)

	if err != nil {
		log.Fatal(err)
	}

	outFile, err := os.Create("out.ppm")
	if err != nil {
		log.Fatal(err)
	}

	err = pnm.Encode(outFile, img, pnm.PPM)
	if err != nil {
		log.Fatal(err)
	}

}
