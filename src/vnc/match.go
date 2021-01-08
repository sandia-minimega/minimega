// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"image"
	"math"
	log "minilog"
	"sync"

	"github.com/anthonynsimon/bild/effect"
	"github.com/anthonynsimon/bild/parallel"
)

func matchTemplate(img, template image.Image) *PointerEvent {
	imgBounds := img.Bounds()
	templateBounds := template.Bounds()

	imgGray := effect.Grayscale(img)
	templateGray := effect.Grayscale(template)

	// find template in img by looking for lowest Sum of Absolute Differences
	var mux sync.Mutex
	minSAD := math.MaxFloat64
	minX := 0
	minY := 0
	normFactor := float64(templateBounds.Dx() * templateBounds.Dy())

	// Note: bild.parallel.Line assumes img.Bounds().Min.X is 0, which should
	// be safe to assume for our freshly-opened PNG, even though Go's image pkg
	// says: An image's bounds do not necessarily start at (0, 0)
	parallel.Line(imgBounds.Dy()-templateBounds.Dy(), func(start, end int) {
		for y := start; y < end; y++ {
			for x := imgBounds.Min.X; x < imgBounds.Max.X-templateBounds.Dx(); x++ {
				var SAD float64

				// loop through the template image
				for j := templateBounds.Min.Y; j < templateBounds.Max.Y; j++ {
					for i := templateBounds.Min.X; i < templateBounds.Max.X; i++ {
						sPos := (y+j)*imgGray.Stride + (x + i)
						s := imgGray.Pix[sPos]
						tPos := j*templateGray.Stride + i
						t := templateGray.Pix[tPos]

						SAD += math.Abs(float64(s)-float64(t)) / normFactor
					}
				}

				// log.Debug("SAD at %v,%v is %v\n", x, y, SAD)

				// save the best found position
				mux.Lock()
				if minSAD > SAD {
					minSAD = SAD
					minX = x
					minY = y
				}
				mux.Unlock()
			}
		}
	})

	log.Debug("lowest SAD %v at %v,%v (center %v,%v)", minSAD, minX, minY, minX+templateBounds.Dx()/2, minY+templateBounds.Dy()/2)

	// 20 seems to be a decent cut off...
	if minSAD > 20.0 {
		return nil
	}

	return &PointerEvent{
		ButtonMask: 1, // left click
		XPosition:  uint16(minX + templateBounds.Dx()/2),
		YPosition:  uint16(minY + templateBounds.Dy()/2),
	}
}
