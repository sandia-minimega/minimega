// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"image"
	"math"
	"sync"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	"github.com/anthonynsimon/bild/effect"
	"github.com/anthonynsimon/bild/parallel"
)

// matchTemplate looks for template in img. If it finds it, it returns a left
// click on the center of the template, in the coordinates of img.
func matchTemplate(img, template image.Image) *PointerEvent {
	imgBounds := img.Bounds()
	templateBounds := template.Bounds()

	w, h := imgBounds.Dx(), imgBounds.Dy()
	tw, th := templateBounds.Dx(), templateBounds.Dy()
	if tw == 0 || th == 0 || tw > w || th > h {
		return nil
	}

	// Grayscale gives RGBA images with R = G = B. Pix[0] is the pixel at
	// Bounds().Min, and each pixel has 4 bytes.
	imgGray := effect.Grayscale(img)
	templateGray := effect.Grayscale(template)

	// find template in img by looking for lowest Sum of Absolute Differences
	var mux sync.Mutex
	minSAD := math.MaxFloat64
	minX := 0
	minY := 0
	normFactor := float64(tw * th)

	parallel.Line(h-th+1, func(start, end int) {
		for y := start; y < end; y++ {
			for x := 0; x <= w-tw; x++ {
				var sum int

				// loop through the template image
				for j := 0; j < th; j++ {
					sRow := imgGray.Pix[(y+j)*imgGray.Stride+x*4:]
					tRow := templateGray.Pix[j*templateGray.Stride:]

					for i := 0; i < tw*4; i += 4 {
						d := int(sRow[i]) - int(tRow[i])
						if d < 0 {
							d = -d
						}
						sum += d
					}
				}

				SAD := float64(sum) / normFactor

				// save the best found position, the first in row order on a tie
				mux.Lock()
				if SAD < minSAD || SAD == minSAD && (y < minY || y == minY && x < minX) {
					minSAD = SAD
					minX = x
					minY = y
				}
				mux.Unlock()
			}
		}
	})

	x := imgBounds.Min.X + minX + tw/2
	y := imgBounds.Min.Y + minY + th/2

	log.Debug("lowest SAD %v at %v,%v (center %v,%v)", minSAD, imgBounds.Min.X+minX, imgBounds.Min.Y+minY, x, y)

	// 20 seems to be a decent cut off...
	if minSAD > 20.0 {
		return nil
	}

	return &PointerEvent{
		ButtonMask: 1, // left click
		XPosition:  uint16(x),
		YPosition:  uint16(y),
	}
}
