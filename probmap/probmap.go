// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package probmap implements a map image
// for a probability density,
// in a plate carrée (equirectangular) projection.
package probmap

import (
	"image"
	"image/color"

	"github.com/js-arias/blind"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/pixkey"
)

type Image struct {
	// Number of columns in the image
	Cols int

	// Age of the time stage of the image
	Age int64

	// Landscape model
	Landscape *model.TimePix

	// Total rotation for the pixels to the present stage
	Tot map[int][]int

	// Color keys
	Keys *pixkey.PixKey

	// Map of Pixels to Probabilities
	Rng map[int]float64

	// Contour image
	Contour image.Image

	// If present is true,
	// it will use the present geography
	Present bool

	// If gray is true,
	// it will use a gray scale.
	Gray bool

	// A Gradient color scheme
	Gradient Gradienter

	step float64
	cAge int64
}

func (i *Image) Format(tot *model.Total) {
	if i.Contour != nil && i.Cols != i.Contour.Bounds().Dx() {
		i.Cols = i.Contour.Bounds().Dx()
	}
	if i.Cols%2 != 0 {
		i.Cols++
	}

	i.step = 360 / float64(i.Cols)
	i.cAge = i.Landscape.ClosestStageAge(i.Age)

	if tot != nil {
		i.Tot = tot.Rotation(i.cAge)
	}

	if i.Gradient == nil {
		i.Gradient = RainbowPurpleToRed{}
	}
}

func (i *Image) ColorModel() color.Model { return color.RGBAModel }
func (i *Image) Bounds() image.Rectangle { return image.Rect(0, 0, i.Cols, i.Cols/2) }
func (i *Image) At(x, y int) color.Color {
	if i.Contour != nil {
		_, _, _, a := i.Contour.At(x, y).RGBA()
		if a > 100 {
			return color.RGBA{A: 255}
		}
	}

	lat := 90 - float64(y)*i.step
	lon := float64(x)*i.step - 180

	pix := i.Landscape.Pixelation().Pixel(lat, lon)

	if i.Tot != nil {
		// Total rotation from present time
		// to stage time
		dst := i.Tot[pix.ID()]
		if len(dst) == 0 {
			v, _ := i.Landscape.At(0, pix.ID())
			if i.Gray {
				if c, ok := i.Keys.Gray(v); ok {
					return c
				}
				return color.RGBA{211, 211, 211, 255}
			}
			if c, ok := i.Keys.Color(v); ok {
				return c
			}
			return color.RGBA{211, 211, 211, 255}
		}

		// Check if the pixel is in the range
		// of the time stage
		var max float64
		for _, px := range dst {
			p := i.Rng[px]
			if p > max {
				max = p
			}
		}
		if max > 0 {
			return i.Gradient.Gradient(max)
		}

		// The taxon is absent,
		// use the landscape value of the pixel
		// at the stage time
		var v int
		if i.Present {
			v, _ = i.Landscape.At(0, pix.ID())
		} else {
			for _, px := range dst {
				vv, _ := i.Landscape.At(i.cAge, px)
				if vv > v {
					v = vv
				}
			}
		}

		if i.Gray {
			if c, ok := i.Keys.Gray(v); ok {
				return c
			}
			return color.RGBA{211, 211, 211, 255}
		}
		if c, ok := i.Keys.Color(v); ok {
			return c
		}
		return color.RGBA{211, 211, 211, 255}
	}

	// No rotation
	if p, ok := i.Rng[pix.ID()]; ok {
		return i.Gradient.Gradient(p)
	}

	v, _ := i.Landscape.At(i.cAge, pix.ID())
	if i.Gray {
		if c, ok := i.Keys.Gray(v); ok {
			return c
		}
		return color.RGBA{211, 211, 211, 255}
	}
	if c, ok := i.Keys.Color(v); ok {
		return c
	}
	return color.RGBA{211, 211, 211, 255}
}

// Gradientes is an interface for types
// that return a color gradient
type Gradienter interface {
	Gradient(v float64) color.Color
}

// HalfGrayScale returns a gray scale
// between 0 (black)
// and 128 (gray).
type HalfGrayScale struct{}

func (h HalfGrayScale) Gradient(v float64) color.Color {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}

	c := 128 - uint8(v*128)
	return color.RGBA{c, c, c, 255}
}

// LightGrayScale returns a gray scale
// between 0 (black)
// to 200 (light gray).
type LightGrayScale struct{}

func (l LightGrayScale) Gradient(v float64) color.Color {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}

	c := 200 - uint8(v*200)
	return color.RGBA{c, c, c, 255}
}

// Incandescent is the incandescent color scheme
// of Paul Tol
// <https://personal.sron.nl/~pault/#fig:scheme_incandescent>.
type Incandescent struct{}

func (i Incandescent) Gradient(v float64) color.Color {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}

	return blind.Sequential(blind.Incandescent, v)
}

// Iridescent is the iridescent color scheme
// of Paul Tol
// <https://personal.sron.nl/~pault/#fig:scheme_iridescent>.
type Iridescent struct{}

func (i Iridescent) Gradient(v float64) color.Color {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}

	return blind.Sequential(blind.Iridescent, v)
}

// RainbowPurpleToRed is the rainbow color scheme
// of Paul Tol
// <https://personal.sron.nl/~pault/#fig:scheme_rainbow_smooth>
// starting at purple and ending at red.
type RainbowPurpleToRed struct{}

func (r RainbowPurpleToRed) Gradient(v float64) color.Color {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}

	return blind.Sequential(blind.RainbowPurpleToRed, v)
}
