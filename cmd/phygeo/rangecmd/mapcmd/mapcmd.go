// Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package mapcmd implements a command to draw
// the geographic range of the taxa in a PhyGeo project
// with defined distribution ranges.
package mapcmd

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"

	"github.com/js-arias/blind"
	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/phygeo/pixkey"
	"github.com/js-arias/phygeo/project"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: `map [-c|--columns <value>] [--key <key-file>] [--gray]
	[-t|--taxon <name>] [--ranges]
	[--unrot] [--present] [--contour <image-file>]
	[-o|--output <file-prefix] <project-file>`,
	Short: "draw a map of the taxa with distribution ranges",
	Long: `
Command map reads the geographic ranges from a PhyGeo project and draws an
image map using a plate carrée (equirectangular) projection.

The argument of the command is the name of the project file.
	
By default, the ranges will be mapped using their respective time stages. If
the flag --unrot is given, then the estimated ranges will be drawn at the
present time. By default, the landscape of the time stage will be used; if the
flag --present is defined, the present landscape will be used for the
background. If the flag --contour is defined with a file, the given image will
be used as a contour of the output map. The contour map will set the size of
the output image and should be fully transparent, except for the contour,
which will always be drawn in black.
	
By default, the output images will be named with the distribution range type
and the taxon name. Use the flag --output, or -o, to set a prefix to each
file.
	
By default, the resulting image will be 3600 pixels wide. Use the flag
--column, or -c, to define a different number of columns. By default, the
images will have a gray background. Use the flag --key to define the landscape
colors of the image. If the flag --gray is set, then gray colors will be used.

By default, map images for all taxa will be produced; use the flag --taxon to
define the map of a particular taxon. By default, maps will be produced using
the ranges defined as points; to make maps based on continuous range maps, use
the flag --ranges.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var grayFlag bool
var unRot bool
var present bool
var rangeFlag bool
var colsFlag int
var contourFile string
var keyFile string
var outPrefix string
var taxFlag string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&grayFlag, "gray", false, "")
	c.Flags().BoolVar(&rangeFlag, "ranges", false, "")
	c.Flags().BoolVar(&unRot, "unrot", false, "")
	c.Flags().BoolVar(&present, "present", false, "")
	c.Flags().IntVar(&colsFlag, "columns", 3600, "")
	c.Flags().IntVar(&colsFlag, "c", 3600, "")
	c.Flags().StringVar(&keyFile, "key", "", "")
	c.Flags().StringVar(&taxFlag, "taxon", "", "")
	c.Flags().StringVar(&taxFlag, "t", "", "")
	c.Flags().StringVar(&outPrefix, "output", "", "")
	c.Flags().StringVar(&outPrefix, "o", "", "")
	c.Flags().StringVar(&contourFile, "contour", "", "")
}

func run(c *command.Command, args []string) error {
	if len(args) < 1 {
		return c.UsageError("expecting project file")
	}

	p, err := project.Read(args[0])
	if err != nil {
		return err
	}

	lsf := p.Path(project.Landscape)
	if lsf == "" {
		msg := fmt.Sprintf("landscape not defined in project %q", args[0])
		return c.UsageError(msg)
	}
	landscape, err := readLandscape(lsf)
	if err != nil {
		return err
	}

	var contour image.Image
	if contourFile != "" {
		contour, err = readContour(contourFile)
		if err != nil {
			return err
		}
		colsFlag = contour.Bounds().Dx()
	}
	if colsFlag%2 != 0 {
		colsFlag++
	}

	var tot *model.Total
	if unRot {
		rotF := p.Path(project.GeoMotion)
		if rotF == "" {
			msg := fmt.Sprintf("plate motion model not defined in project %q", args[0])
			return c.UsageError(msg)
		}
		tot, err = readRotation(rotF, landscape.Pixelation())
		if err != nil {
			return err
		}
	}

	rf := p.Path(project.Points)
	if rangeFlag {
		rf = p.Path(project.Ranges)
	}
	if rf == "" {
		return nil
	}

	coll, err := readRanges(rf)
	if err != nil {
		return err
	}
	if coll == nil {
		return nil
	}

	var keys *pixkey.PixKey
	if keyFile != "" {
		keys, err = pixkey.Read(keyFile)
		if err != nil {
			return err
		}
		if grayFlag && !keys.HasGrayScale() {
			keys = nil
		}
	}

	ls := coll.Taxa()
	if taxFlag != "" {
		taxFlag = strings.ToLower(strings.Join(strings.Fields(taxFlag), " "))
		if !coll.HasTaxon(taxFlag) {
			return nil
		}
		ls = []string{taxFlag}
	}
	for _, tax := range ls {
		age := coll.Age(tax)
		rng := coll.Range(tax)
		out := strings.ToLower(strings.Join(strings.Fields(tax), "_"))
		out = fmt.Sprintf("%s-%s.png", coll.Type(tax), out)
		if outPrefix != "" {
			out = outPrefix + "-" + out
		}

		tm := &taxMap{
			step:      360 / float64(colsFlag),
			age:       age,
			cAge:      landscape.ClosestStageAge(age),
			landscape: landscape,
			keys:      keys,
			rng:       rng,
			contour:   contour,
		}
		if tot != nil {
			tm.tot = tot.Rotation(tm.cAge)
		}

		if err := writeImage(out, tm); err != nil {
			return err
		}
	}

	return nil
}

func readLandscape(name string) (*model.TimePix, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp, err := model.ReadTimePix(f, nil)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return tp, nil
}

func readContour(name string) (image.Image, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("on image file %q: %v", name, err)
	}
	return img, nil
}

func readRotation(name string, pix *earth.Pixelation) (*model.Total, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadTotal(f, pix, false)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return rot, nil
}

func readRanges(name string) (*ranges.Collection, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	coll, err := ranges.ReadTSV(f, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return coll, nil
}

type taxMap struct {
	step      float64
	age       int64
	cAge      int64
	landscape *model.TimePix
	tot       map[int][]int
	keys      *pixkey.PixKey
	rng       map[int]float64

	contour image.Image
}

func (tm *taxMap) ColorModel() color.Model { return color.RGBAModel }
func (tm *taxMap) Bounds() image.Rectangle { return image.Rect(0, 0, colsFlag, colsFlag/2) }
func (tm *taxMap) At(x, y int) color.Color {
	if tm.contour != nil {
		_, _, _, a := tm.contour.At(x, y).RGBA()
		if a > 100 {
			return color.RGBA{A: 255}
		}
	}

	lat := 90 - float64(y)*tm.step
	lon := float64(x)*tm.step - 180

	pix := tm.landscape.Pixelation().Pixel(lat, lon)

	if unRot {
		// Total rotation from present time
		// to stage time
		dst := tm.tot[pix.ID()]
		if len(dst) == 0 {
			v, _ := tm.landscape.At(0, pix.ID())
			if grayFlag {
				if c, ok := tm.keys.Gray(v); ok {
					return c
				}
			} else if tm.keys != nil {
				if c, ok := tm.keys.Color(v); ok {
					return c
				}
			}
			return color.RGBA{211, 211, 211, 255}
		}

		// Check if the pixel is in the range
		// of the time stage
		var max float64
		for _, px := range dst {
			p := tm.rng[px]
			if p > max {
				max = p
			}
		}
		if max > 0 {
			return blind.Gradient(max)
		}

		// The taxon is absent,
		// use the landscape value of the pixel
		// at the stage time
		var v int
		if present {
			v, _ = tm.landscape.At(0, pix.ID())
		} else {
			for _, px := range dst {
				vv, _ := tm.landscape.At(tm.cAge, px)
				if vv > v {
					v = vv
				}
			}
		}

		if grayFlag {
			if c, ok := tm.keys.Gray(v); ok {
				return c
			}
		} else if tm.keys != nil {
			if c, ok := tm.keys.Color(v); ok {
				return c
			}
		}
		return color.RGBA{211, 211, 211, 255}
	}

	if p, ok := tm.rng[pix.ID()]; ok {
		return blind.Gradient(p)
	}

	v, _ := tm.landscape.At(tm.cAge, pix.ID())
	if grayFlag {
		if c, ok := tm.keys.Gray(v); ok {
			return c
		}
	} else if tm.keys != nil {
		if c, ok := tm.keys.Color(v); ok {
			return c
		}
	}
	return color.RGBA{211, 211, 211, 255}
}

func writeImage(name string, m *taxMap) (err error) {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	if err := png.Encode(f, m); err != nil {
		return fmt.Errorf("when encoding image file %q: %v", name, err)
	}
	return nil
}
