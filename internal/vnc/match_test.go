// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"fmt"
	"image"
	"testing"
)

// T15: matchTemplate finds a template at each corner and edge of an image,
// and it gives the center in screen coordinates.
func TestMatchTemplate(t *testing.T) {
	const w, h, tw, th = 40, 30, 6, 5

	full := noiseImage(w+10, h+10, 3)
	screens := map[string]*image.RGBA{
		"origin": noiseImage(w, h, 3),
		"offset": full.SubImage(image.Rect(10, 10, 10+w, 10+h)).(*image.RGBA),
	}

	for name, img := range screens {
		b := img.Bounds()
		for _, pos := range []image.Point{
			{0, 0}, {w - tw, 0}, {0, h - th}, {w - tw, h - th},
			{w / 2, 0}, {0, h / 2}, {w - tw, h / 2}, {w / 2, h - th},
			{w / 3, h / 3},
		} {
			t.Run(fmt.Sprintf("%s %v", name, pos), func(t *testing.T) {
				at := b.Min.Add(pos)
				tmpl := cropCopy(img, image.Rectangle{Min: at, Max: at.Add(image.Pt(tw, th))})

				got := matchTemplate(img, tmpl)
				want := &PointerEvent{ButtonMask: 1, XPosition: uint16(at.X + tw/2), YPosition: uint16(at.Y + th/2)}
				if got == nil || *got != *want {
					t.Errorf("matchTemplate = %+v, want %+v", got, want)
				}
			})
		}
	}

	if got := matchTemplate(screens["origin"], noiseImage(tw, th, 99)); got != nil {
		t.Errorf("matchTemplate of an absent template = %+v, want nil", got)
	}
	if got := matchTemplate(screens["origin"], noiseImage(w+1, h, 3)); got != nil {
		t.Errorf("matchTemplate of a template larger than the image = %+v, want nil", got)
	}
	if got := matchTemplate(screens["origin"], screens["origin"]); got == nil {
		t.Error("matchTemplate of the full image gave no match")
	}
}

// cropCopy returns a copy of r of img with its bounds at 0,0, as a decoded
// template file has.
func cropCopy(img *image.RGBA, r image.Rectangle) *image.RGBA {
	res := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			res.SetRGBA(x, y, img.RGBAAt(r.Min.X+x, r.Min.Y+y))
		}
	}
	return res
}
